package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/33cn/externaldb/erc20scaner/config"
	"github.com/33cn/externaldb/erc20scaner/database"
	"github.com/33cn/externaldb/erc20scaner/logger"
)

var slogger *slog.Logger

var (
	// 全局配置变量，供 tx_parse.go 使用
	globalChainGRPC   string
	globalChainSymbol string
	globalESHost      string
	globalESPrefix    string
	globalESVersion   int
	globalESUser      string
	globalESPassword  string
)

var db *database.DB

func main() {
	// 定义命令行参数
	flags := config.DefineRPCFlags()
	flag.Parse()

	// 加载并合并配置
	cfg, err := config.LoadAndMergeForRPC(flags)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化结构化日志（如果失败则退出程序）
	slogger, err = logger.InitLogger(cfg.Log, "rpc")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// 从已合并的配置中获取值（LoadAndMergeForRPC 已经处理了 flag 优先、配置文件、默认值的逻辑）
	dsn := cfg.Database.DSN
	port := cfg.RPC.Port

	// 设置全局变量供 tx_parse.go 使用
	globalChainGRPC = cfg.Node.GRPC
	globalChainSymbol = cfg.Node.Symbol
	globalESHost = cfg.ES.Host
	globalESPrefix = cfg.ES.Prefix
	globalESVersion = int(cfg.ES.Version)
	globalESUser = cfg.ES.User
	globalESPassword = cfg.ES.Password

	// 连接数据库
	db, err = database.NewDB(dsn)
	if err != nil {
		slogger.Error("Failed to connect to database", "err", err, "dsn", logger.MaskDSN(dsn))
		// 日志此时只写文件（logs/rpc_app.log），容器里只看得到"静默退出"；stderr 补一行 + 非 0 退出
		fmt.Fprintf(os.Stderr, "erc20api: 连接 MySQL 失败: %v（dsn=%s，检查 etc/externaldb.toml 的 [mysql]）\n",
			err, logger.MaskDSN(dsn))
		os.Exit(1)
	}
	defer db.Close()

	slogger.Info("Database connected successfully")
	slogger.Info("Starting HTTP server", "port", port)

	// 注册路由
	http.HandleFunc("/evmapi/contracts", handleContractsRouter)
	http.HandleFunc("/evmapi/contracts/", handleContractsRouter)
	http.HandleFunc("/evmapi/tokens", handleTokensRouter)
	http.HandleFunc("/evmapi/tokens/", handleTokensRouter)
	http.HandleFunc("/evmapi/transactions/", handleTransactionsRouter)
	http.HandleFunc("/evmapi/transactions", handleListTransactions)
	http.HandleFunc("/evmapi/accounts", handleAccountsRouter)
	http.HandleFunc("/evmapi/accounts/", handleAccountsRouter)
	http.HandleFunc("/health", handleHealth)

	// 启动服务器
	addr := fmt.Sprintf(":%s", port)
	slogger.Info("Server listening", "addr", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slogger.Error("Failed to start server", "err", err)
		log.Fatalf("Failed to start server: %v", err)
	}
}
