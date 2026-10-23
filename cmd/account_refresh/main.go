// Package main account_refresh — 定时把链上真实的 BTY 余额校正回 ES 的 account 文档。
//
// 为什么需要：account 文档的 balance/frozen 是日志解析出来的增量，个别情况（缺日志、
// 跨执行器转账、genesis 等）会漂移；而浏览器 TopAsset 排名直接聚合 account.total。
//
// 三档（配置在 [accountRefresh]，间隔可配、<=0 表示该档不跑）：
//   - top   ：total 前 N 的地址（与 TopAsset 同口径），默认 1h
//   - active：近 activeWindow 内在 transaction 出现过的地址（from/to），默认 24h
//   - full  ：全量 account 文档按 balance_refresh_at 升序（缺失优先）每次一页，默认 168h
//
// 只部分更新 balance/frozen/total/balance_refresh_at/balance_refresh_height，
// 不动 height/height_index/block_time/type/addr_type。
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
	chain33types "github.com/33cn/chain33/types"
	tml "github.com/BurntSushi/toml"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/util"
	"github.com/33cn/externaldb/util/accountrefresh"
	"github.com/33cn/externaldb/lib/version"
)

var log = l.New("module", "account_refresh")

var (
	configPath = flag.String("f", "externaldb.toml", "config file")
	tick       = flag.Duration("tick", time.Minute, "内部检查节拍（到期才真正跑；-once 时忽略）")
	once       = flag.Bool("once", false, "只跑一轮（到期档），跑完退出（调试/一次性任务用）")
	dryRun     = flag.Bool("dry-run", false, "只统计要写多少，不实际写 ES")
)

func main() {
	log.Info("account_refresh", "version", version.GetVersion())
	flag.Parse()

	cfg := initCfg(*configPath)
	util.SetupLog("account_refresh", "info")
	if abs, err := filepath.Abs("logs"); err == nil {
		fmt.Fprintf(os.Stderr, "account_refresh: 日志目录 %s\n", abs)
	}

	rc := cfg.GetAccountRefresh()
	if rc == nil || !rc.Enabled {
		log.Info("account_refresh disabled（[accountRefresh].enabled=false），退出")
		return
	}
	db.SetVersion(cfg.GetEsVersion())
	db.SetAddrID(cfg.GetConvert().GetAddressDriver())

	es, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		fatal("ES 连接失败（检查 [convertEs] host/user/pwd）", err)
	}

	conn, err := grpc.Dial(cfg.Chain.GrpcHost,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		fatal("连接 chain33 gRPC 失败（检查 [chain].grpcHost）", err)
	}
	defer conn.Close()

	opt, err := buildOptions(cfg, es, accountrefresh.NewGRPCSource(chain33types.NewChain33Client(conn)))
	if err != nil {
		fatal("配置解析失败（[accountRefresh]）", err)
	}
	if *dryRun {
		opt.DryRun = true
	}
	opt.Log = func(msg string, kv ...interface{}) { log.Info(msg, kv...) }

	r := accountrefresh.New(opt)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *once {
		st, err := r.RunOnce(ctx)
		if err != nil {
			fatal("account_refresh 一轮失败", err)
		}
		log.Info("account_refresh done",
			"top", st.Top, "active", st.Active, "full", st.Full,
			"changed", st.Changed, "dropped", st.Dropped)
		return
	}
	log.Info("account_refresh loop start",
		"topEvery", opt.TopInterval, "activeEvery", opt.ActiveInterval, "fullEvery", opt.FullInterval,
		"topN", opt.TopN, "batch", opt.BatchSize, "page", opt.PageSize,
		"ticketExec", opt.TicketExec, "dryRun", opt.DryRun)
	if err := r.Loop(ctx, *tick); err != nil {
		fatal("account_refresh 退出", err)
	}
	log.Info("account_refresh stopped")
}

// buildOptions 把 [accountRefresh] 配置转成刷新参数（未配置项用内置默认）
func buildOptions(cfg *proto.ConfigNew, es escli.ESClient, chain accountrefresh.BalanceSource) (accountrefresh.Options, error) {
	rc := cfg.GetAccountRefresh()
	opt := accountrefresh.Options{
		ES:     es,
		Chain:  chain,
		Symbol: rc.GetSymbol(),
		Title:  cfg.GetChain().GetTitle(),

		TopN:      int(rc.GetTopN()),
		PageSize:  int(rc.GetPageSize()),
		BatchSize: int(rc.GetBatchSize()),

		TopInterval:    parseDur(rc.GetTopInterval(), time.Hour),
		ActiveInterval: parseDur(rc.GetActiveInterval(), 24*time.Hour),
		FullInterval:   parseDur(rc.GetFullInterval(), 168*time.Hour),
		ActiveWindow:   parseDur(rc.GetActiveWindow(), 24*time.Hour),
		TicketExec:     rc.GetTicketExec(),
	}
	if opt.Symbol == "" {
		opt.Symbol = cfg.GetChain().GetSymbol()
	}
	if opt.Symbol == "" {
		return opt, fmt.Errorf("[accountRefresh].symbol 与 [chain].symbol 都为空")
	}
	return opt, nil
}

func parseDur(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Warn("解析时长失败，用默认值", "value", s, "default", def, "err", err)
		return def
	}
	return d
}

func initCfg(path string) *proto.ConfigNew {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: init config failed path=%s err=%v\n", path, err)
		os.Exit(1)
	}
	return &cfg
}

func fatal(msg string, err error) {
	log.Error(msg, "err", err)
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", msg, err)
	os.Exit(1)
}
