// scannerfix 是一个数据修复工具，按 seq 范围扫描区块。
// 与 scanner 不同的是：
//   - -s (start seq) 和 -e (end seq) 是必填参数，不填退出
//   - 不读/写 scan_progress 表（方案 D），不影响线上 scanner 的断点续扫
//   - 处理进度仅通过日志输出，默认日志文件 scannerfix_app.log
//   - 到达 end seq 后自动退出
//   - -chain-grpc 指定 chain33 gRPC 地址启用 chain33 模式，否则使用 ETH 节点模式
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	chain33types "github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/erc20Scaner/config"
	"github.com/33cn/externaldb/erc20Scaner/logger"
	"github.com/33cn/externaldb/erc20Scaner/scanner/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 自定义 flags：-s 和 -e 必填
	configFile := flag.String("c", "config.yaml", "config file path")
	startSeq := flag.Int64("s", 0, "start seq (REQUIRED)")
	endSeq := flag.Int64("e", 0, "end seq (REQUIRED)")
	nodeURL := flag.String("u", "", "node url (overrides config)")
	dbEnabled := flag.Bool("db", false, "enable database (overrides config)")
	dbDSN := flag.String("dsn", "", "database DSN (overrides config)")
	chainGRPC := flag.String("chain-grpc", "", "chain33 gRPC host (enables chain33 mode)")
	chainSymbol := flag.String("chain-symbol", "", "Chain33 symbol (overrides config)")
	skipInlineBalanceUpdate := flag.Bool("skip-inline-balance", false, "skip inline balanceOf (overrides config)")
	flag.Parse()

	// 验证必填参数
	if *startSeq <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: -s (start seq) is required and must be > 0")
		fmt.Fprintln(os.Stderr, "Usage: scannerfix -s <start_seq> -e <end_seq> [-c config.yaml] [-u node_url] [-db -dsn dsn] [-chain-grpc host]")
		os.Exit(1)
	}
	if *endSeq <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: -e (end seq) is required and must be > 0")
		fmt.Fprintln(os.Stderr, "Usage: scannerfix -s <start_seq> -e <end_seq> [-c config.yaml] [-u node_url] [-db -dsn dsn] [-chain-grpc host]")
		os.Exit(1)
	}
	if *startSeq >= *endSeq {
		fmt.Fprintf(os.Stderr, "ERROR: -s (%d) must be less than -e (%d)\n", *startSeq, *endSeq)
		os.Exit(1)
	}

	// 加载配置文件（如果存在）
	var cfg *config.Config
	if _, err := os.Stat(*configFile); os.IsNotExist(err) {
		cfg = config.GetDefaultConfig()
	} else {
		var loadErr error
		cfg, loadErr = config.LoadConfig(*configFile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load config file: %v, using defaults\n", loadErr)
			cfg = config.GetDefaultConfig()
		}
	}

	// 覆盖配置（命令行 > 配置文件）
	if *nodeURL != "" {
		cfg.Node.URL = *nodeURL
	}
	if *dbEnabled {
		cfg.Database.Enabled = *dbEnabled
	}
	if *dbDSN != "" {
		cfg.Database.DSN = *dbDSN
	}
	if *chainGRPC != "" {
		cfg.Node.GRPC = *chainGRPC
	}
	if *chainSymbol != "" {
		cfg.Node.Symbol = *chainSymbol
	}
	if *skipInlineBalanceUpdate {
		cfg.Scanner.SkipInlineBalanceUpdate = *skipInlineBalanceUpdate
	}

	// 使用 scannerfix 专用日志文件名
	cfg.Log.File = "./logs/scannerfix_app.log"

	// 初始化日志
	log, err := logger.InitLogger(cfg.Log, "scannerfix")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// 设置 engine 包的 logger
	engine.SetLogger(log)

	log.Info("=== ScannerFix Started ===",
		"startSeq", *startSeq,
		"endSeq", *endSeq,
		"nodeURL", cfg.Node.URL,
		"dbEnabled", cfg.Database.Enabled,
		"chain33Mode", cfg.Node.GRPC != "",
		"mode", "fix (no scan_progress read/write)")

	// 创建 Process 进入修复模式
	p := new(engine.Process)
	p.StartPoint = uint64(*startSeq)
	p.EndPoint = uint64(*endSeq)
	p.EnableDB = cfg.Database.Enabled
	p.DBDSN = cfg.Database.DSN
	p.NodeURL = cfg.Node.URL
	p.SkipInlineBalanceUpdate = cfg.Scanner.SkipInlineBalanceUpdate
	p.NoProgress = true // 方案 D：不读/写 scan_progress

	if cfg.Node.GRPC != "" {
		log.Info("chain33 mode enabled", "grpc", cfg.Node.GRPC)
		grpcConn, err := grpc.Dial(cfg.Node.GRPC,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
		)
		if err != nil {
			log.Error("Failed to connect to chain33 gRPC", "err", err, "addr", cfg.Node.GRPC)
			os.Exit(1)
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
		log.Info("Node mode enabled", "url", cfg.Node.URL)
		p.Init()
		if cfg.Database.Enabled && cfg.BalanceRefresher.Enabled {
			go p.RunBalanceRefresher(context.Background(), cfg.BalanceRefresher)
		}
		defer p.Close()
		p.Start()
	}

	log.Info("=== ScannerFix Exited ===")
}
