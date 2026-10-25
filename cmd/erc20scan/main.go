package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	chain33types "github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/erc20/config"
	"github.com/33cn/externaldb/erc20/logger"
	"github.com/33cn/externaldb/erc20/scanner/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"log/slog"
)

var log *slog.Logger

func main() {
	// 定义命令行参数
	flags := config.DefineScannerFlags()
	flag.Parse()

	// 校验运行模式（-mode run|fix）：fix 模式要求显式 -s/-e，校验失败直接退出
	fixMode, err := flags.FixMode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}

	// 加载并合并配置
	cfg, err := config.LoadAndMergeForScanner(flags)
	if err != nil {
		// 使用 fmt 输出，因为 log 还未初始化
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// 初始化日志（如果失败则退出程序）。fix 模式单独一个日志文件，避免和线上 scanner 混在一起。
	appName := "scanner"
	if fixMode {
		appName = "scannerfix"
	}
	log, err = logger.InitLogger(cfg.Log, appName)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		return
	}

	// 设置 engine 包的 logger
	engine.SetLogger(log)

	// 打印配置信息
	logConfig(cfg, log, fixMode)

	// 初始化并启动
	initAndStart(cfg, fixMode)
}

func initAndStart(cfg *config.Config, fixMode bool) {
	p := new(engine.Process)
	p.StartPoint = uint64(cfg.Scanner.StartBlock)
	p.EndPoint = uint64(cfg.Scanner.EndBlock)
	p.EnableDB = cfg.Database.Enabled
	p.DBDSN = cfg.Database.DSN
	p.NodeURL = cfg.Node.URL
	p.SkipInlineBalanceUpdate = cfg.Scanner.SkipInlineBalanceUpdate
	p.FixMode = fixMode // fix 模式：不读/写进度，处理完 [s,e] 即退出

	if fixMode {
		log.Info("[Init] fix mode", "from", p.StartPoint, "to", p.EndPoint,
			"note", "不读/写 scan_progress、scan_seq_marker；处理完即退出")
		if !cfg.Database.Enabled {
			log.Warn("[Init] database disabled, fix mode will only parse and print (nothing written)")
		}
	}

	// 启动自检 eth JSON-RPC：失败直接给可读错误退出，避免 engine 里的 panic 让进程"静默退出"
	if err := engine.PreflightEthRPC(cfg.Node.URL); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: eth JSON-RPC 不可用: %v\n", err)
		os.Exit(1)
	}

	// 如果启用了 chain33 模式，优先使用 gRPC 按 seq 读取区块
	if cfg.ES.Enabled {
		log.Info("[Init] chain33 mode enabled", "grpc", cfg.Node.GRPC)
		grpcConn, err := grpc.Dial(cfg.Node.GRPC,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
		)
		if err != nil {
			log.Error("[Init] Failed to connect to chain33 gRPC", "err", err, "addr", cfg.Node.GRPC)
			return
		}
		defer grpcConn.Close()
		grpcClient := chain33types.NewChain33Client(grpcConn)
		log.Info("[Init] chain33 gRPC connection established")

		// fix 模式预检：-e 超出节点当前最大 seq 时直接报错，避免修复进程一直空转等待
		if fixMode {
			checkFixRange(grpcClient, p.StartPoint, p.EndPoint)
		}

		p.Init()
		if cfg.Database.Enabled && cfg.BalanceRefresher.Enabled && !fixMode {
			go p.RunBalanceRefresher(context.Background(), cfg.BalanceRefresher)
		}
		defer p.Close()
		p.StartWithChain33(grpcClient, grpcConn)
	} else {
		// 使用节点模式
		log.Info("[Init] Node mode enabled", "url", cfg.Node.URL)
		if fixMode {
			checkFixRangeNode(cfg.Node.URL, p.EndPoint)
		}
		p.Init()
		if cfg.Database.Enabled && cfg.BalanceRefresher.Enabled && !fixMode {
			go p.RunBalanceRefresher(context.Background(), cfg.BalanceRefresher)
		}
		defer p.Close()
		p.Start()
	}
}

// checkFixRange 校验修复区间没有超出节点已有范围。
// 拿不到节点最大 seq 时只告警继续（节点可能没开 isRecordBlockSequence，但 GetBlockBySeq 仍可用）。
func checkFixRange(cli chain33types.Chain33Client, from, to uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	last, err := cli.GetLastBlockSequence(ctx, &chain33types.ReqNil{})
	if err != nil {
		log.Warn("[Init] fix mode: query last seq failed, skip range precheck", "err", err)
		return
	}
	if uint64(last.GetData()) < to {
		log.Error("[Init] fix mode: end seq is ahead of node",
			"to", to, "lastSeq", last.GetData())
		fmt.Fprintf(os.Stderr, "FATAL: fix 模式 -e=%d 超出节点当前最大 seq=%d\n", to, last.GetData())
		os.Exit(1)
	}
	log.Info("[Init] fix mode: range precheck ok", "from", from, "to", to, "lastSeq", last.GetData())
}

// checkFixRangeNode 节点模式（eth JSON-RPC）下校验 -e 没有超出当前高度。
func checkFixRangeNode(nodeURL string, to uint64) {
	c := new(engine.Client)
	c.ConnectEth(nodeURL)
	defer c.CloseConnect()
	head, err := c.BlockNum()
	if err != nil {
		log.Warn("[Init] fix mode: query block number failed, skip range precheck", "err", err)
		return
	}
	if head < to {
		log.Error("[Init] fix mode: end block is ahead of node", "to", to, "head", head)
		fmt.Fprintf(os.Stderr, "FATAL: fix 模式 -e=%d 超出节点当前高度=%d\n", to, head)
		os.Exit(1)
	}
	log.Info("[Init] fix mode: range precheck ok", "to", to, "head", head)
}

func logConfig(cfg *config.Config, log *slog.Logger, fixMode bool) {
	mode := config.ScannerModeRun
	if fixMode {
		mode = config.ScannerModeFix
	}
	log.Info("=== Configuration ===",
		"mode", mode,
		"nodeURL", cfg.Node.URL,
		"startBlock", cfg.Scanner.StartBlock,
		"endBlock", cfg.Scanner.EndBlock,
		"dbEnabled", cfg.Database.Enabled,
		"chain33Mode", cfg.ES.Enabled,
		"skipInlineBalanceUpdate", cfg.Scanner.SkipInlineBalanceUpdate,
		"balanceRefresherEnabled", cfg.BalanceRefresher.Enabled)

	if cfg.Database.Enabled {
		log.Info("Database configuration", "dsn", logger.MaskDSN(cfg.Database.DSN))
	}
	if cfg.BalanceRefresher.Enabled {
		log.Info("balance_refresher configuration",
			"interval", cfg.BalanceRefresher.Interval,
			"batch_size", cfg.BalanceRefresher.BatchSize,
			"concurrency", cfg.BalanceRefresher.Concurrency,
			"min_age", cfg.BalanceRefresher.MinAge)
	}
	if cfg.ES.Enabled {
		log.Info("chain33 gRPC configuration",
			"grpc", cfg.Node.GRPC)
	}
}
