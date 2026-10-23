// Package main pull_convert — convert 直连节点（pull + seq 驱动）。
//
// 数据流:
//
//	chain33 (gRPC GetBlockSequences + GetBlockByHashes) → convert → ES
//	                          ↑                                ↑
//	                   按 seq 主动拉取                    seq_marker（游标 = max(seq)）
//
// 与旧链路（sync → seq05_seq → convert）的区别：不再把整块 seq 存 ES，
// 拉到数据后直接解析；进度是 append-only 的 seq marker（见 db/seqmark），恢复时取 max(seq)。
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

	l "github.com/33cn/chain33/common/log/log15"
	_ "github.com/33cn/chain33/system"
	_ "github.com/33cn/plugin/plugin/dapp/init"
	_ "github.com/33cn/plugin/plugin/store/init"
	tml "github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/escli"
	"github.com/33cn/externaldb/lib/esdoc/seqmark"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/lib/util"
	"github.com/33cn/externaldb/lib/version"

	// 注册所有 converter（同 cmd/convert/main.go）
	_ "github.com/33cn/externaldb/convert/coins"
	_ "github.com/33cn/externaldb/convert/coinsx"
	_ "github.com/33cn/externaldb/convert/evm"
	_ "github.com/33cn/externaldb/convert/evm/erc1155"
	_ "github.com/33cn/externaldb/convert/evm/erc721"
	_ "github.com/33cn/externaldb/convert/evm/nft"
	_ "github.com/33cn/externaldb/convert/evmxgo"
	_ "github.com/33cn/externaldb/convert/filepart"
	_ "github.com/33cn/externaldb/convert/filesummary"
	_ "github.com/33cn/externaldb/convert/multisig"
	pipeline "github.com/33cn/externaldb/convert/pipeline"
	_ "github.com/33cn/externaldb/convert/pos33"
	_ "github.com/33cn/externaldb/convert/proof"
	_ "github.com/33cn/externaldb/convert/proof_config"
	_ "github.com/33cn/externaldb/convert/stat/block"
	_ "github.com/33cn/externaldb/convert/ticket"
	_ "github.com/33cn/externaldb/convert/token"
	_ "github.com/33cn/externaldb/convert/trade"
	_ "github.com/33cn/externaldb/convert/unfreeze"
	_ "github.com/33cn/externaldb/lib/esdoc/account"
	esmaint "github.com/33cn/externaldb/lib/esmaint"
)

var (
	log        = l.New("module", "pull_convert")
	configPath = flag.String("f", "externaldb.toml", "config file")
	chainPath  = flag.String("c", "", "chain33 node config file（v1.72 起 fork 严格校验，必须给完整配置）")
	fromSeq    = flag.Int64("from", -1, "run: 强制从该 seq 之后开始（>=0 时忽略 marker）；fix: 修复区间的起始 seq（含）")
	toSeq      = flag.Int64("to", -1, "fix 模式的结束 seq（含）；run 模式忽略")
	mode       = flag.String("mode", "run", "运行模式：run=跟随链尖；fix=只修复 -from..-to 区间后退出（不写 seq_marker、不做统计累计）")
)

func main() {
	log.Info("pull_convert", "version", version.GetVersion())
	flag.Parse()

	fixMode := *mode == "fix"
	if *mode != "run" && !fixMode {
		fmt.Fprintf(os.Stderr, "FATAL: -mode 只支持 run/fix（收到 %q）\n", *mode)
		os.Exit(1)
	}
	if fixMode {
		if *fromSeq < 0 || *toSeq < *fromSeq {
			fmt.Fprintf(os.Stderr, "FATAL: fix 模式要求 -from >= 0 且 -to >= -from（收到 from=%d to=%d）\n", *fromSeq, *toSeq)
			os.Exit(1)
		}
	} else if *toSeq >= 0 {
		fmt.Fprintln(os.Stderr, "WARN: -to 只在 -mode fix 下生效，run 模式忽略")
	}

	cfg := initCfg(*configPath)
	title := cfg.Chain.Title
	if title == "" {
		title = "bityuan"
	}
	symbol := cfg.Chain.Symbol
	if symbol == "" {
		symbol = "bty"
	}
	chaincfg.InitChain33(title, symbol, *chainPath)
	util.SetupLog(cfg.Convert.GetAppName(), "debug")
	// SetupLog 之后 log15 只写文件（logs/<appName>.log 与 .error.log），控制台不再有日志；
	// 这里显式提示一次目录与 appName，并让后续启动期致命错误同时打 stderr（见 fatal）。
	if abs, aerr := filepath.Abs("logs"); aerr == nil {
		fmt.Fprintf(os.Stderr, "pull_convert: 日志目录 %s（appName=%s，之后日志只写文件）\n",
			abs, cfg.Convert.GetAppName())
	}
	esmaint.InitMapSet(cfg.EsVersion, cfg.EsIndex)
	schema.SetVersion(cfg.EsVersion)

	workDir := pwd()
	if err := os.Chdir(workDir); err != nil {
		fatal("chdir failed", "dir", workDir, "err", err)
	}
	log.Info("work dir", "dir", workDir)

	// 1. ES 连接
	// 先自己连一次做自检：InitWriteDB 内部失败是 panic，认证失败只会打一坨堆栈。
	esClient, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		fatal("ES 连接失败：检查 etc/externaldb.toml 的 [convertEs] host/user/pwd 是否与 ES 实际账号一致"+
			"（开了 xpack.security 时是 elastic / .env 里的 ES_PASSWORD；注意 ES 数据目录若是旧的，"+
			"改 .env 里的 ES_PASSWORD 不会跟着改密码）",
			"host", cfg.ConvertEs.Host, "user", cfg.ConvertEs.User, "err", err)
	}
	// 索引初始化（converter 的 InitDB 走 pipeline.EsWrite）；ES 已确认可连，不会再 panic
	pipeline.InitWriteDB(cfg.ConvertEs, cfg.EsVersion)
	chaincfg.SetAddrID(cfg.Convert.AddressDriver)
	pipeline.InitContract(cfg)
	pipeline.InitConfig(cfg)
	app := pipeline.NewApp(cfg)
	if !fixMode {
		if err := seqmark.InitDB(esClient); err != nil {
			fatal("seqmark.InitDB failed", "err", err)
		}
		// marker 写入用普通 bulk（不再逐 seq 等 refresh）；这里显式 refresh 一次再读游标，
		// 保证"已 ACK 但还没被 ES 自动 refresh"的 marker 对 search 可见（否则重启会多重放几条）。
		if err := seqmark.Refresh(esClient); err != nil {
			log.Warn("seqmark 索引 refresh 失败，游标可能读到旧值（会重放少量 seq）", "err", err)
		}
	}

	// 2. 节点 seq 来源
	src, err := pipeline.NewGRPCSource(cfg.Chain.GrpcHost)
	if err != nil {
		fatal("connect chain33 failed", "grpcHost", cfg.Chain.GrpcHost, "err", err)
	}
	defer func() { _ = src.Close() }()

	// 3. 游标：fix 模式用 -from/-to 划区间，不读 marker、不动统计；run 模式按 marker/旧进度自举
	var cursor int64
	if fixMode {
		cursor = *fromSeq - 1 // 主循环从 cursor+1 开始，即 [from, to] 闭区间
		log.Info("fix 模式", "from", *fromSeq, "to", *toSeq,
			"note", "不写 seq_marker、不做 block_stat 累计；完成后用 convert_check -from/-to 对账")
	} else {
		var cursorHeight int64
		var err error
		cursor, cursorHeight, err = resolveCursor(src, esClient, cfg, *fromSeq)
		if err != nil {
			fatal("启动失败", "err", err)
		}
		log.Info("progress ready", "cursor", cursor, "cursorHeight", cursorHeight)
		// 累计统计（block_stat 的 prev）必须与恢复点对齐：按游标块高度恢复，
		// 否则"数据已写、marker 未写"的崩溃重放会把同一块重复累加（见 stat.HeightRecover）
		if cursorHeight >= 0 {
			if err := app.RecoverStatsAt(esClient, cursorHeight); err != nil {
				fatal("RecoverStats(At height) failed", "height", cursorHeight, "err", err)
			}
			log.Info("RecoverStats done", "atHeight", cursorHeight)
		} else {
			log.Warn("游标高度未知（无 marker/旧进度），只能按全局最新恢复统计；若统计值异常请从有 marker 的点重启")
			if err := app.RecoverStats(esClient, cursor); err != nil {
				fatal("RecoverStats failed", "err", err)
			}
		}
	}

	// 4. 主循环
	mod := &util.ModuleConvert{
		Name:       cfg.Convert.AppName,
		WriteDB:    esClient,
		StartSeq:   cfg.Convert.StartSeq,
		AppConvert: app,
	}
	opt := pipeline.PullOptions{
		Cursor:       cursor,
		Batch:        int(cfg.Convert.PullBatch),
		PollInterval: time.Duration(cfg.Convert.PullPollMs) * time.Millisecond,
		MarkerKeep:   cfg.Convert.PullMarkerKeep,
	}
	if fixMode {
		opt.FixMode = true
		opt.EndSeq = *toSeq
		opt.SkipMarker = true
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := pipeline.RunPull(ctx, src, pipeline.NewESWriter(esClient), mod,
		pipeline.NewDetailCache(int(cfg.Convert.PullDetailCache)), opt); err != nil {
		fatal("pull_convert 异常退出", "err", err)
	}
	log.Info("pull_convert stopped")
}

// resolveCursor 启动探测 + 游标初始化。
//
//   - 探测节点 seq 能力：GetLastBlockSequence 必须可用（节点需开启 isRecordBlockSequence 且从 0 高度起完整）；
//     再用最后一个 seq 自检 GetBlockSequences/GetBlockByHashes 能取到块。
//   - 起点优先级见 pipeline.ResolveCursor：-from → 已有 marker（与节点比对）→ 旧 last_seq 自举 → startSeq-1。
func resolveCursor(src pipeline.SeqSource, esClient escli.ESClient, cfg *proto.ConfigNew, fromSeq int64) (int64, int64, error) {
	last, err := src.LastSeq()
	if err != nil {
		return 0, 0, errors.Wrap(err, "探测节点 seq 失败（确认 blockchain.isRecordBlockSequence=true 且从 0 高度起完整）")
	}
	if last < 0 {
		return 0, 0, errors.New("探测节点 seq 失败：节点 seq 日志为空")
	}
	if _, err := src.Batch(last, last); err != nil {
		return 0, 0, errors.Wrapf(err, "探测节点 seq 失败：读取 seq(%d) 出错", last)
	}
	log.Info("node seq probe ok", "lastSeq", last)

	seq, marker, err := seqmark.Max(esClient)
	if err != nil {
		return 0, 0, err
	}
	// 兼容切换：读旧 convert 的进度（doc id = [convert].appName），没有则 -1
	oldSeq, err := esmaint.LastSyncSeq(esClient, cfg.Convert.AppName)
	if err != nil {
		log.Warn("读取旧 db05_last_seq 失败，忽略", "appName", cfg.Convert.AppName, "err", err)
		oldSeq = -1
	}

	return pipeline.ResolveCursor(src, pipeline.CursorSources{
		MarkerSeq: seq,
		Marker:    marker,
		OldSeq:    oldSeq,
		StartSeq:  cfg.Convert.StartSeq,
		SaveMarker: func(m *seqmark.Marker) error {
			return seqmark.Save(esClient, []*seqmark.Marker{m})
		},
	}, fromSeq, func(msg string, ctx ...interface{}) {
		log.Warn(msg, ctx...)
	})
}

// fatal 启动期致命错误：log 可能已重定向到文件，这里同时打 stderr，避免"静默退出"。
func fatal(msg string, kv ...interface{}) {
	log.Error(msg, kv...)
	fmt.Fprintf(os.Stderr, "FATAL: %s %v\n", msg, kv)
	os.Exit(1)
}

func initCfg(path string) *proto.ConfigNew {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		log.Error("init config failed", "path", path, "err", err)
		fmt.Fprintf(os.Stderr, "FATAL: init config failed path=%s err=%v\n", path, err)
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
