package main

import (
	"context"
	"flag"
	"fmt"

	chain33types "github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/erc20Scaner/config"
	"github.com/33cn/externaldb/erc20Scaner/logger"
	"github.com/33cn/externaldb/erc20Scaner/scanner/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"log/slog"
)

var log *slog.Logger

func main() {
	// 定义命令行参数
	flags := config.DefineScannerFlags()
	flag.Parse()

	// 加载并合并配置
	cfg, err := config.LoadAndMergeForScanner(flags)
	if err != nil {
		// 使用 fmt 输出，因为 log 还未初始化
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	// 初始化日志（如果失败则退出程序）
	log, err = logger.InitLogger(cfg.Log, "scanner")
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		return
	}

	// 设置 engine 包的 logger
	engine.SetLogger(log)

	// 打印配置信息
	logConfig(cfg, log)

	// 初始化并启动
	initAndStart(cfg)
}

func initAndStart(cfg *config.Config) {
	p := new(engine.Process)
	p.StartPoint = uint64(cfg.Scanner.StartBlock)
	p.EndPoint = uint64(cfg.Scanner.EndBlock)
	p.EnableDB = cfg.Database.Enabled
	p.DBDSN = cfg.Database.DSN
	p.NodeURL = cfg.Node.URL
	p.SkipInlineBalanceUpdate = cfg.Scanner.SkipInlineBalanceUpdate

	// 如果启用了 chain33 模式，优先使用 gRPC 按 seq 读取区块
	if cfg.ES.Enabled {
		log.Info("chain33 mode enabled", "grpc", cfg.Node.GRPC)
		grpcConn, err := grpc.Dial(cfg.Node.GRPC,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
		)
		if err != nil {
			log.Error("Failed to connect to chain33 gRPC", "err", err, "addr", cfg.Node.GRPC)
			return
		}
		defer grpcConn.Close()
		grpcClient := chain33types.NewChain33Client(grpcConn)
		log.Info("chain33 gRPC connection established successfully")
		p.Init()
		if cfg.Database.Enabled && cfg.BalanceRefresher.Enabled {
			go p.RunBalanceRefresher(context.Background(), cfg.BalanceRefresher)
		}
		defer p.Close()
		p.StartWithChain33(grpcClient, grpcConn)
	} else {
		// 使用节点模式
		log.Info("Node mode enabled", "url", cfg.Node.URL)
		p.Init()
		if cfg.Database.Enabled && cfg.BalanceRefresher.Enabled {
			go p.RunBalanceRefresher(context.Background(), cfg.BalanceRefresher)
		}
		defer p.Close()
		p.Start()
	}
}

func logConfig(cfg *config.Config, log *slog.Logger) {
	log.Info("=== Configuration ===",
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
