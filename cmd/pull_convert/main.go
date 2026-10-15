// Package main pull_convert — convert 直连节点（pull + seq 驱动）。
//
// 数据流:
//
//	chain33 (gRPC GetBlockSequences + GetBlockByHashes) → convert → ES
//	                          ↑                                ↑
//	                   按 seq 主动拉取                    seq_marker（游标 = max(seq)）
//
// 与旧链路（sync → seq05_seq → convert）的区别：不再把整块 seq 存 ES，
// 拉到数据后直接解析；进度是 append-only 的 seq marker，恢复时取 max(seq)。
// 设计见 docs/requirements/convert-direct-from-node.md。
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	l "github.com/33cn/chain33/common/log/log15"
	_ "github.com/33cn/chain33/system"
	_ "github.com/33cn/plugin/plugin/dapp/init"
	_ "github.com/33cn/plugin/plugin/store/init"
	"github.com/pkg/errors"
	tml "github.com/BurntSushi/toml"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/seqmark"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/proto"
	"github.com/33cn/externaldb/util"
	"github.com/33cn/externaldb/util/cli/convert"
	"github.com/33cn/externaldb/version"

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
	log        = l.New("module", "pull_convert")
	configPath = flag.String("f", "externaldb.toml", "config file")
	chainPath  = flag.String("c", "", "chain33 node config file（v1.72 起 fork 严格校验，必须给完整配置）")
	fromSeq    = flag.Int64("from", -1, "强制从该 seq 之后开始（>=0 时忽略 marker，用于修复/回放）")
)

func main() {
	log.Info("pull_convert", "version", version.GetVersion())
	flag.Parse()

	cfg := initCfg(*configPath)
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

	workDir := pwd()
	if err := os.Chdir(workDir); err != nil {
		log.Error("chdir failed", "dir", workDir, "err", err)
		os.Exit(1)
	}
	log.Info("work dir", "dir", workDir)

	// 1. ES 连接 + 各类索引初始化（converter 的 InitDB 走 convert.EsWrite）
	convert.InitWriteDB(cfg.ConvertEs, cfg.EsVersion)
	esClient, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		log.Error("ES connect failed", "host", cfg.ConvertEs.Host, "prefix", cfg.ConvertEs.Prefix, "err", err)
		os.Exit(1)
	}
	db.SetAddrID(cfg.Convert.AddressDriver)
	convert.InitContract(cfg)
	convert.InitConfig(cfg)
	app := convert.NewApp(cfg)
	if err := seqmark.InitDB(esClient); err != nil {
		log.Error("seqmark.InitDB failed", "err", err)
		os.Exit(1)
	}

	// 2. 节点 seq 来源
	src, err := convert.NewGRPCSource(cfg.Chain.GrpcHost)
	if err != nil {
		log.Error("connect chain33 failed", "grpcHost", cfg.Chain.GrpcHost, "err", err)
		os.Exit(1)
	}
	defer func() { _ = src.Close() }()

	// 3. 启动探测 + 游标（marker → 与节点比对）
	cursor, err := resolveCursor(src, esClient, cfg, *fromSeq)
	if err != nil {
		log.Error("启动失败", "err", err)
		os.Exit(1)
	}
	log.Info("progress ready", "cursor", cursor)
	if err := app.RecoverStats(esClient, cursor); err != nil {
		log.Error("RecoverStats failed", "err", err)
		os.Exit(1)
	}

	// 4. 主循环
	mod := &util.ModuleConvert{
		Name:        cfg.Convert.AppName,
		WriteDB:     esClient,
		StartSeq:    cfg.Convert.StartSeq,
		AppConvert:  app,
	}
	opt := convert.PullOptions{
		Cursor:       cursor,
		Batch:        int(cfg.Convert.PullBatch),
		PollInterval: time.Duration(cfg.Convert.PullPollMs) * time.Millisecond,
		MarkerKeep:   cfg.Convert.PullMarkerKeep,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := convert.RunPull(ctx, src, convert.NewESWriter(esClient), mod,
		convert.NewDetailCache(int(cfg.Convert.PullDetailCache)), opt); err != nil {
		log.Error("pull_convert 异常退出", "err", err)
		os.Exit(1)
	}
	log.Info("pull_convert stopped")
}

// resolveCursor 启动探测 + 游标初始化 + 一致性校验。
//
//   - 探测节点 seq 能力：GetLastBlockSequence 必须可用（节点需开启 isRecordBlockSequence 且从 0 高度起完整）；
//     再用最后一个 seq 自检 GetBlockSequences/GetBlockByHashes 能取到块。
//   - fromSeq >= 0：显式指定起点（修复/回放），不做 marker 校验。
//   - 否则取 seq_marker 的 max(seq)；无 marker 时用 [convert].startSeq-1。
//   - 有 marker 时把它的 (hash,type) 与节点同 seq 的记录比对，不一致直接退出
//     （换过节点 / 节点 seq 日志被重建，人工确认后再处理）。
func resolveCursor(src convert.SeqSource, esClient escli.ESClient, cfg *proto.ConfigNew, fromSeq int64) (int64, error) {
	last, err := src.LastSeq()
	if err != nil {
		return 0, errors.Wrap(err, "探测节点 seq 失败（确认 blockchain.isRecordBlockSequence=true 且从 0 高度起完整）")
	}
	if last < 0 {
		return 0, errors.New("探测节点 seq 失败：节点 seq 日志为空")
	}
	if _, err := src.Batch(last, last); err != nil {
		return 0, errors.Wrapf(err, "探测节点 seq 失败：读取 seq(%d) 出错", last)
	}
	log.Info("node seq probe ok", "lastSeq", last)

	if fromSeq >= 0 {
		log.Warn("忽略 marker，使用 -from 指定的起点", "from", fromSeq)
		return fromSeq, nil
	}

	seq, m, err := seqmark.Max(esClient)
	if err != nil {
		return 0, err
	}
	if m == nil {
		cursor := cfg.Convert.StartSeq - 1
		log.Info("无 seq marker，按配置 startSeq 起跑", "startSeq", cfg.Convert.StartSeq, "cursor", cursor)
		return cursor, nil
	}

	cur, err := src.Batch(seq, seq)
	if err != nil {
		return 0, errors.Wrapf(err, "校验游标失败：读取 seq(%d) 出错", seq)
	}
	if len(cur) != 1 {
		return 0, errors.Errorf("校验游标失败：marker 的 seq(%d) 在节点 seq 日志中不存在", seq)
	}
	if cur[0].Hash != m.Hash || cur[0].Type != m.Type {
		return 0, errors.Errorf(
			"游标与节点不一致：seq=%d marker(hash=%s,type=%d) node(hash=%s,type=%d)。"+
				"可能换了节点或节点 seq 日志被重建，请人工确认后再处理",
			seq, m.Hash, m.Type, cur[0].Hash, cur[0].Type)
	}
	return seq, nil
}

func initCfg(path string) *proto.ConfigNew {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		log.Error("init config failed", "path", path, "err", err)
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
