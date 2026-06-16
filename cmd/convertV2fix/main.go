// Package main convertV2fix — convertV2 修复工具，按 seq 范围处理指定区间后退出。
//
// 与 convertV2 不同的是：
//   - -s (start seq) 和 -e (end seq) 是必填参数
//   - 处理完区间后自动退出，不持续监听
//   - 使用独立的进度文件 (data/last_convertfix)，不影响 convertV2 进度
//   - 不调用 RecoverStats
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/33cn/chain33/common"
	l "github.com/33cn/chain33/common/log/log15"
	_ "github.com/33cn/chain33/system"
	"github.com/33cn/chain33/types"
	_ "github.com/33cn/plugin/plugin/dapp/init"
	_ "github.com/33cn/plugin/plugin/store/init"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/block"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/proto"
	"github.com/33cn/externaldb/store/syncseq"
	"github.com/33cn/externaldb/util"
	"github.com/33cn/externaldb/util/cli/convert"
	"github.com/33cn/externaldb/util/localfile"
	"github.com/33cn/externaldb/version"
	tml "github.com/BurntSushi/toml"

	_ "github.com/33cn/externaldb/db/account"
	_ "github.com/33cn/externaldb/db/coins"
	_ "github.com/33cn/externaldb/db/evm"
	_ "github.com/33cn/externaldb/db/evm/erc1155"
	_ "github.com/33cn/externaldb/db/evm/erc721"
	_ "github.com/33cn/externaldb/db/evm/nft"
	_ "github.com/33cn/externaldb/db/evmxgo"
	_ "github.com/33cn/externaldb/db/filepart"
	_ "github.com/33cn/externaldb/db/filesummary"
	_ "github.com/33cn/externaldb/db/multisig"
	_ "github.com/33cn/externaldb/db/proof"
	_ "github.com/33cn/externaldb/db/proof_config"
	_ "github.com/33cn/externaldb/db/ticket"
	_ "github.com/33cn/externaldb/db/token"
	_ "github.com/33cn/externaldb/db/trade"
	_ "github.com/33cn/externaldb/db/unfreeze"
	_ "github.com/33cn/externaldb/stat/block"
)

var (
	log        = l.New("module", "convertV2fix")
	configPath = flag.String("f", "externaldb.toml", "config file")
	chainPath  = flag.String("c", "", "chain33 node config file")
	startSeq   = flag.Int64("s", 0, "start seq (REQUIRED)")
	endSeq     = flag.Int64("e", 0, "end seq (REQUIRED)")
	batchSize  = flag.Int("batch", 100, "batch size for gRPC block fetch")
)

// 进度文件名（与 convertV2 的 last_convert 区分）
const fixProgressFile = "last_convertfix"

func main() {
	log.Info("convertV2fix", "version", version.GetVersion())

	flag.Parse()
	cfg := initCfg(*configPath)

	if *startSeq <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: -s (start seq) is required and must be > 0")
		os.Exit(1)
	}
	if *endSeq <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: -e (end seq) is required and must be > 0")
		os.Exit(1)
	}
	if *startSeq >= *endSeq {
		fmt.Fprintf(os.Stderr, "ERROR: -s (%d) must be less than -e (%d)\n", *startSeq, *endSeq)
		os.Exit(1)
	}

	log.Info("[Init] convertV2fix", "startSeq", *startSeq, "endSeq", *endSeq, "batch", *batchSize)

	// 初始化 chain33
	title := cfg.Chain.Title
	if title == "" {
		title = "bityuan"
	}
	symbol := cfg.Chain.Symbol
	if symbol == "" {
		symbol = "bty"
	}
	util.InitChain33(title, symbol, *chainPath)
	util.SetupLog("convertV2fix", "debug")
	util.InitMapSet(cfg.EsVersion, cfg.EsIndex)
	db.SetVersion(cfg.EsVersion)

	workDir := pwd()
	os.Chdir(workDir)
	log.Info("[Init] work dir", "dir", workDir)

	// 连接 ES
	esClient, err := escli.NewESLongConnect(
		cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd,
	)
	if err != nil {
		log.Error("[Init] ES connect failed", "err", err)
		os.Exit(1)
	}

	// 连接 chain33 gRPC
	grpcAddr := cfg.Chain.GrpcHost
	if grpcAddr == "" {
		log.Error("[Init] chain.grpcHost is empty")
		os.Exit(1)
	}
	grpcConn, err := grpc.Dial(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		log.Error("[Init] gRPC dial failed", "addr", grpcAddr, "err", err)
		os.Exit(1)
	}
	defer grpcConn.Close()
	grpcClient := types.NewChain33Client(grpcConn)
	log.Info("[Init] gRPC connected", "addr", grpcAddr)

	// 初始化 converter
	convert.InitDB(cfg)
	app := convert.NewApp(cfg)

	// 修复模式：独立的进度文件
	progressPath := syncseq.ProgressFilePath(workDir, fixProgressFile)
	progressFP, err := localfile.NewFileProgress(progressPath)
	if err != nil {
		log.Error("[Init] NewFileProgress failed", "err", err)
		os.Exit(1)
	}
	resumeSeq, err := progressFP.Load()
	if err != nil {
		log.Error("[Init] progressFP.Load failed", "err", err)
		os.Exit(1)
	}
	if resumeSeq < *startSeq-1 {
		resumeSeq = *startSeq - 1
	}
	log.Info("[Init] progress ready", "path", progressPath, "resumeSeq", resumeSeq)

	// 启动处理
	shutdown := make(chan struct{})
	go func() {
		runFixLoop(grpcClient, app, esClient, progressFP, resumeSeq, *startSeq, *endSeq, *batchSize, shutdown)
	}()
	gracefulCloseFix(shutdown)
}

// runFixLoop 按 seq 范围处理区块，处理完退出。
func runFixLoop(
	cli types.Chain33Client,
	app *convert.App,
	esClient escli.ESClient,
	progressFP *localfile.FileProgress,
	resumeSeq, start, end int64, batch int,
	shutdown <-chan struct{},
) {
	current := resumeSeq

	for current < end {
		select {
		case <-shutdown:
			log.Info("[Exit] signal received, saving progress", "seq", current)
			progressFP.Save(current)
			return
		default:
		}

		if current < start-1 {
			current = start - 1
		}

		endSeq := current + int64(batch)
		if endSeq > end {
			endSeq = end
		}

		log.Info("[Round Begin] fixing blocks", "startSeq", current+1, "endSeq", endSeq)

		blocks, err := fetchBlockDetails(cli, current+1, endSeq)
		if err != nil {
			log.Error("[Round] fetchBlockDetails failed", "err", err, "startSeq", current+1, "endSeq", endSeq)
			time.Sleep(3 * time.Second)
			continue
		}
		if len(blocks) == 0 {
			log.Info("[Round] no blocks returned", "startSeq", current+1, "endSeq", endSeq)
			current = endSeq
			progressFP.Save(current)
			continue
		}

		firstH := blocks[0].Detail.Block.GetHeight()
		lastH := blocks[len(blocks)-1].Detail.Block.GetHeight()
		log.Info("[Round] blocks fetched", "seqRange", fmt.Sprintf("%d-%d", current+1, endSeq),
			"heightRange", fmt.Sprintf("%d-%d", firstH, lastH), "count", len(blocks))

		var allRecords []db.Record
		for _, bd := range blocks {
			records, convErr := app.ConvertBlock(bd.Seq, bd.Detail)
			if convErr != nil {
				log.Error("[Round] ConvertBlock failed", "err", convErr, "height", bd.Detail.Block.Height, "seq", bd.Seq.SyncSeq)
				continue
			}
			allRecords = append(allRecords, records...)
		}

		if len(allRecords) == 0 {
			log.Info("[Round] no records produced", "seqRange", fmt.Sprintf("%d-%d", current+1, endSeq))
			current = endSeq
			progressFP.Save(current)
			continue
		}

		if saveErr := util.SaveToES(esClient, allRecords); saveErr != nil {
			log.Error("[Round] SaveToES failed", "err", saveErr)
			time.Sleep(3 * time.Second)
			continue
		}

		if saveErr := progressFP.Save(endSeq); saveErr != nil {
			log.Error("[Round] progressFP.Save failed", "err", saveErr, "seq", endSeq)
			time.Sleep(1 * time.Second)
			continue
		}
		current = endSeq

		log.Info("[Round End] batch completed", "seq", current, "height", lastH, "remaining", end-current)
	}

	log.Info("[Exit] fix completed", "start", start, "end", end, "lastSeq", current)
}

// fetchBlockDetails from convertV2: GetBlockSequences + GetBlockByHashes
func fetchBlockDetails(cli types.Chain33Client, startSeq, endSeq int64) ([]*blockData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. seq 元数据
	seqReply, err := cli.GetBlockSequences(ctx, &types.ReqBlocks{Start: startSeq, End: endSeq})
	if err != nil {
		return nil, err
	}
	seqItems := seqReply.GetItems()
	if len(seqItems) == 0 {
		return nil, nil
	}

	// 2. 收集 hashes
	hashes := make([][]byte, len(seqItems))
	for i, item := range seqItems {
		hashes[i] = item.GetHash()
	}

	// 3. 批量获取区块详情
	blockReply, err := cli.GetBlockByHashes(ctx, &types.ReqHashes{Hashes: hashes})
	if err != nil {
		return nil, err
	}
	if blockReply == nil {
		return nil, nil
	}
	detailItems := blockReply.GetItems()

	if len(seqItems) != len(detailItems) {
		log.Warn("seq/detail count mismatch", "seqItems", len(seqItems), "detailItems", len(detailItems))
	}

	minLen := len(seqItems)
	if len(detailItems) < minLen {
		minLen = len(detailItems)
	}

	var results []*blockData
	for i := 0; i < minLen; i++ {
		seqItem := seqItems[i]
		detail := detailItems[i]

		hashHex := common.ToHex(seqItem.GetHash())
		seqNum := startSeq + int64(i)
		height := detail.Block.GetHeight()

		bs := &block.Seq{
			SyncSeq:     int(seqNum),
			From:        "grpc-direct",
			Number:      int(height),
			Hash:        hashHex,
			Type:        int(seqItem.GetType()),
			BlockDetail: types.Encode(detail),
		}

		results = append(results, &blockData{Seq: bs, Detail: detail})
	}

	return results, nil
}

type blockData struct {
	Seq    *block.Seq
	Detail *types.BlockDetail
}

func initCfg(path string) *proto.ConfigNew {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		log.Error("init config failed", "err", err)
		os.Exit(1)
	}
	return &cfg
}

func pwd() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}
	return dir
}

func gracefulCloseFix(shutdown chan<- struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigs
	log.Info("[Exit] convertV2fix shutting down", "signal", sig)
	close(shutdown)
	time.Sleep(3 * time.Second)
	log.Info("[Exit] convertV2fix stopped")
}
