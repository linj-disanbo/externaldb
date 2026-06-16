// Package main convertV2 — convert 直连 chain33 节点版本。
//
// 数据流:
//
//	chain33 (gRPC GetBlockSequences + GetBlockByHashes) → convert → ES
//	                             ↑
//	                       本地文件进度 (data/last_convert)
//
// 不再依赖 sync 程序做 ES 中转。
package main

import (
	"context"
	"encoding/json"
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
	"github.com/33cn/externaldb/escli/querypara"
	"github.com/33cn/externaldb/proto"
	"github.com/33cn/externaldb/store/syncseq"
	"github.com/33cn/externaldb/util"
	"github.com/33cn/externaldb/util/cli/convert"
	"github.com/33cn/externaldb/util/localfile"
	"github.com/33cn/externaldb/version"
	tml "github.com/BurntSushi/toml"

	// 注册所有 converter（同 cmd/convert/main.go）
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
	log        = l.New("module", "convertV2")
	configPath = flag.String("f", "externaldb.toml", "config file")
	chainPath  = flag.String("c", "", "chain33 node config file")
	batchSize  = flag.Int("batch", 100, "batch size for gRPC block fetch")
)

func main() {
	log.Info("convertV2", "version", version.GetVersion())

	flag.Parse()
	cfg := initCfg(*configPath)

	// 初始化 chain33 类型系统
	title := cfg.Chain.Title
	if title == "" {
		title = "bityuan"
	}
	symbol := cfg.Chain.Symbol
	if symbol == "" {
		symbol = "bty"
	}
	util.InitChain33(title, symbol, *chainPath)
	util.SetupLog(cfg.Convert.GetAppName(), "debug")
	util.InitMapSet(cfg.EsVersion, cfg.EsIndex)
	db.SetVersion(cfg.EsVersion)

	// 获取工作目录
	workDir := pwd()
	os.Chdir(workDir)
	log.Info("work dir", "dir", workDir)

	// 1. 连接 ES（写入）
	esClient, err := escli.NewESLongConnect(
		cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd,
	)
	if err != nil {
		log.Error("ES connect failed", "err", err)
		os.Exit(1)
	}

	// 2. 连接 chain33 gRPC
	grpcAddr := cfg.Chain.GrpcHost
	if grpcAddr == "" {
		log.Error("chain.grpcHost is empty, need gRPC address")
		os.Exit(1)
	}
	grpcConn, err := grpc.Dial(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		log.Error("gRPC dial failed", "addr", grpcAddr, "err", err)
		os.Exit(1)
	}
	defer grpcConn.Close()
	grpcClient := types.NewChain33Client(grpcConn)
	log.Info("gRPC connected", "addr", grpcAddr)

	// 3. 初始化 converter
	convert.InitDB(cfg)
	app := convert.NewApp(cfg)

	// 4. 进度恢复：本地文件 + ES 交叉验证
	progressPath := syncseq.ProgressFilePath(workDir, syncseq.DefaultConvertProgressFile)
	progressFP, err := localfile.NewFileProgress(progressPath)
	if err != nil {
		log.Error("NewFileProgress failed", "err", err)
		os.Exit(1)
	}

	lastProcessed, err := validateOrDeriveProgress(progressFP, esClient, cfg, grpcClient)
	if err != nil {
		log.Error("progress validation failed", "err", err)
		os.Exit(1)
	}
	log.Info("[Init] progress ready", "path", progressPath, "lastProcessed", lastProcessed)

	// 5. 恢复统计计数器（同原 convert 的 RecoverStats）
	if err := app.RecoverStats(esClient, lastProcessed); err != nil {
		log.Error("[Init] RecoverStats failed", "err", err)
		os.Exit(1)
	}
	log.Info("[Init] RecoverStats done", "lastProcessed", lastProcessed)

	// 6. 启动主循环
	shutdown := make(chan struct{})
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		runConvertLoop(grpcClient, app, esClient, progressFP, lastProcessed, *batchSize, shutdown)
	}()

	// 7. 等待退出信号
	gracefulClose(shutdown, loopDone)
}

// runConvertLoop 主循环：从节点拉取区块 → convert → 写 ES → 更新进度。
// lastProcessed 是上次已处理完成的 seq（seq >= height，回滚后 seq > height），循环从 lastProcessed+1 开始。
func runConvertLoop(
	cli types.Chain33Client,
	app *convert.App,
	esClient escli.ESClient,
	progressFP *localfile.FileProgress,
	lastProcessed int64,
	batch int,
	shutdown <-chan struct{},
) {
	current := lastProcessed

	for {
		// 检查退出信号
		select {
		case <-shutdown:
			log.Info("[Exit] signal received, saving progress", "seq", current)
			if saveErr := progressFP.Save(current); saveErr != nil {
				log.Error("[Exit] progressFP.Save failed", "err", saveErr, "seq", current)
			}
			return
		default:
		}
		// 获取链最新 seq（GetLastBlockSequence）
		latestSeq, err := getLatestSeq(cli)
		if err != nil {
			log.Error("[Round] getLatestSeq failed", "err", err)
			time.Sleep(3 * time.Second)
			continue
		}

		if current >= latestSeq {
			time.Sleep(3 * time.Second)
			continue
		}

		// 批量拉取（按 seq 范围，不依赖 height）
		endSeq := current + int64(batch)
		if endSeq > latestSeq {
			endSeq = latestSeq
		}

		log.Info("[Round Begin] fetching blocks", "startSeq", current+1, "endSeq", endSeq, "latestSeq", latestSeq)

		blocks, err := fetchBlockDetails(cli, current+1, endSeq)
		if err != nil {
			log.Error("[Round] fetchBlockDetails failed", "err", err, "startSeq", current+1, "endSeq", endSeq)
			time.Sleep(3 * time.Second)
			continue
		}
		if len(blocks) == 0 {
			log.Info("[Round] no blocks returned", "startSeq", current+1, "endSeq", endSeq)
			// 节点返回空（可能是尚未出块），保存进度避免重启重复拉取
			current = endSeq
			if saveErr := progressFP.Save(current); saveErr != nil {
				log.Error("progressFP.Save failed", "err", saveErr)
			}
			continue
		}

		// 打印 seq→height 映射，便于追踪回滚导致的 seq/height 偏差
		firstH := blocks[0].Detail.Block.GetHeight()
		lastH := blocks[len(blocks)-1].Detail.Block.GetHeight()
		log.Info("[Round] blocks fetched", "seqRange", fmt.Sprintf("%d-%d", current+1, endSeq),
			"heightRange", fmt.Sprintf("%d-%d", firstH, lastH), "count", len(blocks))

		// 转换每个区块
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
			log.Info("[Round] no records produced", "seqRange", fmt.Sprintf("%d-%d", current+1, endSeq),
				"heightRange", fmt.Sprintf("%d-%d", firstH, lastH))
			current = endSeq
			if saveErr := progressFP.Save(current); saveErr != nil {
				log.Error("progressFP.Save failed", "err", saveErr)
			}
			continue
		}

		// 写入 ES
		if saveErr := util.SaveToES(esClient, allRecords); saveErr != nil {
			log.Error("[Round] SaveToES failed", "err", saveErr, "seqRange", fmt.Sprintf("%d-%d", current+1, endSeq))
			time.Sleep(3 * time.Second)
			continue
		}

		// 先保存进度文件，成功后再更新内存中的 current。
		// 如果保存失败，保持 current 不变，下轮重试。
		if saveErr := progressFP.Save(endSeq); saveErr != nil {
			log.Error("[Round] progressFP.Save failed, will retry", "err", saveErr, "seq", endSeq)
			time.Sleep(1 * time.Second)
			continue
		}
		current = endSeq

		log.Info("[Round End] batch completed", "seq", current, "height", lastH, "latestSeq", latestSeq)
	}
}

// blockData 封装区块 seq 元数据和区块详情，供 ConvertBlock 使用。
type blockData struct {
	Seq    *block.Seq
	Detail *types.BlockDetail
}

// fetchBlockDetails 通过 gRPC 批量获取区块详情。
// 分两步，不依赖 seq==height 的假设：
//  1. GetBlockSequences(startSeq, endSeq) → 获取 seq 元数据（hash, type），按 seq 排序
//  2. GetBlockByHashes(hashes) → 按 hash 批量获取完整区块详情
// 两步按 hash 对应（输入输出顺序一致），seq 从 startSeq+i 推导。
func fetchBlockDetails(cli types.Chain33Client, startSeq, endSeq int64) ([]*blockData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 获取 seq 元数据（hash, type），按 seq 顺序返回
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

	// 3. 按 hashes 批量获取完整区块详情
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

	// 4. 按索引匹配（GetBlockSequences 和 GetBlockByHashes 都保持输入顺序）
	minLen := len(seqItems)
	if len(detailItems) < minLen {
		minLen = len(detailItems)
	}

	var results []*blockData
	for i := 0; i < minLen; i++ {
		seqItem := seqItems[i]
		detail := detailItems[i]

		hashHex := common.ToHex(seqItem.GetHash())
		seqNum := startSeq + int64(i) // 真实的 seq 号，seq >= height
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

// getLatestSeq 获取链最新区块 seq（GetLastBlockSequence）。
// seq >= height，回滚后 seq 继续递增而 height 回退。
func getLatestSeq(cli types.Chain33Client) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reply, err := cli.GetLastBlockSequence(ctx, &types.ReqNil{})
	if err != nil {
		return 0, err
	}
	return reply.GetData(), nil
}

// esHeightToSeq 将 ES 中获取的 height 转换为链上的 seq。
// 正确做法：GetBlockHash(height) → hash → GetSequenceByHash(hash) → seq。
// 失败时退化假设 seq==height（主链场景）。
func esHeightToSeq(cli types.Chain33Client, height int64) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 按 height 获取 block hash
	hashReply, err := cli.GetBlockHash(ctx, &types.ReqInt{Height: height})
	if err != nil || hashReply == nil {
		log.Info("esHeightToSeq: GetBlockHash failed, assuming seq==height", "height", height, "err", err)
		return height
	}

	// 2. 按 hash 获取 seq
	seqReply, err := cli.GetSequenceByHash(ctx, &types.ReqHash{Hash: hashReply.GetHash()})
	if err != nil || seqReply == nil {
		log.Info("esHeightToSeq: GetSequenceByHash failed, assuming seq==height", "height", height, "err", err)
		return height
	}

	seq := seqReply.GetData()
	if seq != height {
		log.Info("esHeightToSeq: seq != height",
			"esHeight", height, "seq", seq, "delta", seq-height)
	}
	return seq
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

// validateOrDeriveProgress 进度恢复与验证。
//
// 场景处理：
//  1. 文件存在且内容有效 → 与 ES 交叉验证：
//     a. 文件 > ES → 不可能，文件可能损坏 → 退出
//     b. ES 远大于文件 → 文件可能是旧备份 → 退出
//     c. 一致或在容差范围内 → 使用文件值
//  2. 文件不存在 → 尝试从 ES 推导：
//     a. ES 有数据 → 使用 ES 最大值，写入文件
//     b. ES 无数据 → 使用配置 startSeq，写入文件
//  3. 文件存在但解析失败 → 尝试从 ES 推导（同场景 2）
func validateOrDeriveProgress(
	fp *localfile.FileProgress,
	esClient escli.ESClient,
	cfg *proto.ConfigNew,
	cli types.Chain33Client,
) (int64, error) {
	fileSeq, fileErr := fp.Load()
	fileAvailable := fileErr == nil

	// ES 返回的是 height，需要转换为 seq 才能和文件比较
	esHeight, esFound, esErr := deriveMaxHeightFromES(esClient, cfg.Convert.SaveBlockInfo)
	if esErr != nil {
		log.Info("ES progress query failed (non-fatal)", "err", esErr)
	}
	var esSeq int64
	if esFound {
		esSeq = esHeightToSeq(cli, esHeight)
		log.Info("ES height converted to seq", "esHeight", esHeight, "esSeq", esSeq)
	}

	// 场景 1: 文件可用 → 交叉验证
	// 文件存在说明之前运行过，fileSeq 必定 >= StartSeq-1，不需要 clamp
	if fileAvailable {
		log.Info("progress file found", "fileSeq", fileSeq)

		if esFound {
			gap := esSeq - fileSeq
			switch {
			case fileSeq > esSeq:
				// 文件声称处理到了 ES 没有的 seq → 文件损坏或被人为修改
				return 0, fmt.Errorf(
					"PROGRESS MISMATCH: file says seq=%d but ES max seq is %d (height=%d). "+
						"The progress file may be corrupted. "+
						"To reset progress, delete this file: %s",
					fileSeq, esSeq, esHeight, fp.Path())
			case gap > int64(*batchSize):
				// 正常运行时 ES 写入后立刻保存文件，gap 不会超过一个 batch
				// 超过说明文件可能是旧备份
				return 0, fmt.Errorf(
					"PROGRESS MISMATCH: file says seq=%d but ES has data up to seq=%d (height=%d, gap=%d > batch=%d). "+
						"The progress file may be from an old backup. "+
						"To continue from ES state, delete: %s",
					fileSeq, esSeq, esHeight, gap, *batchSize, fp.Path())
			default:
				log.Info("progress validated against ES",
					"fileSeq", fileSeq, "esSeq", esSeq, "esHeight", esHeight, "gap", gap)
			}
		} else {
			log.Info("ES has no block data, trusting progress file")
		}
		return fileSeq, nil
	}

	// 文件不可用（fileErr != nil）
	log.Info("progress file unavailable", "err", fileErr, "path", fp.Path())

	// 场景 2/3: 尝试从 ES 推导（ES 的值是 height，已转为 seq）
	if esFound {
		log.Warn("deriving progress from ES data", "esHeight", esHeight, "esSeq", esSeq)
		if saveErr := fp.Save(esSeq); saveErr != nil {
			return 0, fmt.Errorf("failed to save ES-derived progress: %v", saveErr)
		}
		return esSeq, nil
	}

	// ES 也没有数据 → 使用配置起始值
	fallback := cfg.Convert.StartSeq - 1
	log.Warn("no progress source available, using config startSeq",
		"startSeq", cfg.Convert.StartSeq, "fallback", fallback)
	if saveErr := fp.Save(fallback); saveErr != nil {
		return 0, fmt.Errorf("failed to save fallback progress: %v", saveErr)
	}
	return fallback, nil
}

// deriveMaxHeightFromES 从 ES 的 block_info 索引中获取已处理的最大区块高度。
// 返回 (maxHeight, found, error)。
func deriveMaxHeightFromES(esClient escli.ESClient, saveBlockInfo bool) (int64, bool, error) {
	if !saveBlockInfo {
		log.Info("SaveBlockInfo is disabled, ES cross-validation unavailable")
		return 0, false, nil
	}

	query := &querypara.Query{
		Sort: []*querypara.QSort{{Key: "height", Ascending: false}},
		Size: &querypara.QSize{Size: 1},
	}

	var maxHeight int64 = -1
	decode := func(x *json.RawMessage) (interface{}, error) {
		var doc struct {
			Height int64 `json:"height"`
		}
		if err := json.Unmarshal(*x, &doc); err != nil {
			return nil, err
		}
		return doc.Height, nil
	}

	results, err := esClient.Search("block_info", "block_info", query, decode)
	if err != nil {
		// 索引可能不存在或查询失败
		return 0, false, nil
	}
	if len(results) > 0 {
		if h, ok := results[0].(int64); ok && h > 0 {
			maxHeight = h
		}
	}

	if maxHeight < 0 {
		return 0, false, nil
	}
	return maxHeight, true, nil
}

func gracefulClose(shutdown chan<- struct{}, loopDone <-chan struct{}) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigs
	log.Info("[Exit] convertV2 shutting down", "signal", sig)

	// 通知主循环退出
	close(shutdown)

	// 等待主循环完成，超时 30s
	select {
	case <-loopDone:
		log.Info("[Exit] loop exited cleanly")
	case <-time.After(30 * time.Second):
		log.Warn("[Exit] loop did not exit within 30s, forcing shutdown")
	}
	log.Info("[Exit] convertV2 stopped")
}
