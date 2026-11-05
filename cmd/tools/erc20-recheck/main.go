// erc20-recheck 一次性回填工具：修复 2026-11-05 事故（eth_call 字段名不匹配）留下的
//   - contracts.contract_type = 'UNKNOWN'（判定失败/从未判定）
//   - token_balances 空表（内联 balanceOf 全部失败，一行都没写）
//
// 判定口径与 erc20/scanner/engine 保持一致：eth_getCode 有代码 + 字节码里能找到
// "63"+selector 形式的 4/6 个核心选择器 + totalSupply() 能调通 → ERC20。
// eth_call 同时写 data 和 input（chain33 只认 data，geth 1.14 只发 input）。
//
// 用法：
//
//	erc20-recheck -dsn 'root:pwd@tcp(127.0.0.1:3306)/token_scanner?charset=utf8mb4&parseTime=true' \
//	              -rpc http://127.0.0.1:8546 -stage contracts -dry-run
//	... -stage contracts   # 只回填合约类型/名称/符号/精度/总量
//	... -stage balances    # 只重建 token_balances（需 contracts 已是 ERC20）
//	... -stage all         # 两个阶段顺序执行（默认）
//
// 只写 contracts 与 token_balances 两张表，不动 events / transactions / token_allowances /
// 扫块进度；可重复执行（幂等）。
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	_ "github.com/go-sql-driver/mysql"

	"github.com/33cn/externaldb/erc20/config"
	"github.com/33cn/externaldb/erc20/database"
	"github.com/33cn/externaldb/erc20/erc20abi/generated"
	"github.com/33cn/externaldb/erc20/logger"
)

const zeroAddr = "0x0000000000000000000000000000000000000000"

// 与 erc20/scanner/engine/process.go 的 checkERC20BySelectorUncached 保持一致
var (
	coreSelectors = []string{
		"18160ddd", // totalSupply()
		"70a08231", // balanceOf(address)
		"a9059cbb", // transfer(address,uint256)
		"23b872dd", // transferFrom(address,address,uint256)
		"095ea7b3", // approve(address,uint256)
		"dd62ed3e", // allowance(address,address)
	}
	verifiedFuncsJSON = `["18160ddd","70a08231","a9059cbb","23b872dd","095ea7b3","dd62ed3e"]`
	maxDecimal65      = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(65), nil), big.NewInt(1))
)

type options struct {
	configPath  string
	dsn         string
	rpcURL      string
	stage       string
	concurrency int
	limit       int
	dryRun      bool
	onlyUnknown bool
	pairsLimit  int
	retries     int
	funcs       string
	fromBlock   int64
	afterID     int64
	batchSize   int
	verbose     bool
}

func main() {
	var opt options
	flag.StringVar(&opt.configPath, "c", "", "externaldb.toml 路径（从 [mysql].dsn 与 [erc20scan].ethUrl 取值；也可用 -dsn/-rpc 覆盖）")
	flag.StringVar(&opt.dsn, "dsn", "", "MySQL DSN（-c 没给时必填）")
	flag.StringVar(&opt.rpcURL, "rpc", "", "eth JSON-RPC URL（-c 没给时必填）")
	flag.StringVar(&opt.stage, "stage", "all", "contracts|balances|prune-eoa|backfill-value|all")
	flag.IntVar(&opt.concurrency, "concurrency", 20, "并发数（RPC 查询）")
	flag.IntVar(&opt.limit, "limit", 0, "本次最多处理多少行（0=全部；contracts/prune-eoa 按地址、backfill-value 按交易，页级近似）")
	flag.IntVar(&opt.pairsLimit, "pairs-limit", 0, "余额阶段最多处理多少个 (地址,合约) 对（0=全部）")
	flag.BoolVar(&opt.dryRun, "dry-run", false, "只打印将要写入的内容，不落库")
	flag.BoolVar(&opt.onlyUnknown, "only-unknown", true, "合约阶段只处理 contract_type='UNKNOWN'")
	flag.BoolVar(&opt.verbose, "v", false, "打印每个地址的判定结果")
	flag.IntVar(&opt.retries, "retries", 2, "瞬时错误（超时/限流/网络）重试次数")
	flag.StringVar(&opt.funcs, "func", "eth_transfer", "backfill-value 只补这些 func_name（逗号分隔，all=不限）")
	flag.Int64Var(&opt.fromBlock, "from-block", 0, "backfill-value 只补 >= 该高度的交易（0=不限）")
	flag.Int64Var(&opt.afterID, "after-id", 0, "backfill-value 从 id > 该值 开始（断点续跑用，结束时会打印下次的值）")
	flag.IntVar(&opt.batchSize, "batch-size", 2000, "backfill-value 每页取多少行（内部游标分批，避免每轮重扫）")
	flag.Parse()

	if opt.configPath != "" {
		cfg, err := config.LoadConfigFile(opt.configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: 读取配置 %s 失败: %v\n", opt.configPath, err)
			os.Exit(1)
		}
		if opt.dsn == "" {
			opt.dsn = cfg.Database.DSN
		}
		if opt.rpcURL == "" {
			opt.rpcURL = cfg.Node.URL
		}
		if !cfg.Database.Enabled {
			fmt.Fprintln(os.Stderr, "WARN: 配置里 [mysql].enabled=false，仍按 dsn 连库")
		}
	}
	if opt.dsn == "" || opt.rpcURL == "" {
		fmt.Fprintf(os.Stderr, "FATAL: 缺少 %s（用 -c etc/externaldb.toml，或显式 -dsn/-rpc）\n",
			strings.Join(missing(opt), " 与 "))
		fmt.Fprintf(os.Stderr, "\n用法: %s -c etc/externaldb.toml [-stage contracts|balances|all] [-dry-run]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "      %s -dsn '<mysql dsn>' -rpc '<eth json-rpc>' ...\n\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}
	if opt.stage != "contracts" && opt.stage != "balances" && opt.stage != "prune-eoa" &&
		opt.stage != "backfill-value" && opt.stage != "all" {
		fmt.Fprintf(os.Stderr, "FATAL: -stage 只支持 contracts/balances/prune-eoa/backfill-value/all（收到 %q）\n", opt.stage)
		os.Exit(2)
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := sql.Open("mysql", opt.dsn)
	if err != nil {
		log.Error("open mysql", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	// 连接池：与并发数对齐，并让连接定期重建。
	// 线上 `unexpected EOF` + `invalid connection` 的典型来源是池里的连接被服务端
	// （wait_timeout / 前置代理空闲超时 / 重启）关掉了，拿旧连接就直接报错。
	db.SetMaxOpenConns(opt.concurrency)
	db.SetMaxIdleConns(opt.concurrency)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	if err := db.PingContext(context.Background()); err != nil {
		log.Error("connect mysql", "err", err)
		os.Exit(1)
	}
	var waitTimeout, netWrite, netRead, maxConn, lockWait sql.NullInt64
	_ = db.QueryRowContext(context.Background(),
		`SELECT @@wait_timeout, @@net_write_timeout, @@net_read_timeout, @@max_connections, @@innodb_lock_wait_timeout`).
		Scan(&waitTimeout, &netWrite, &netRead, &maxConn, &lockWait)
	log.Info("mysql session", "wait_timeout", waitTimeout.Int64, "net_write_timeout", netWrite.Int64,
		"net_read_timeout", netRead.Int64, "max_connections", maxConn.Int64,
		"innodb_lock_wait_timeout", lockWait.Int64)

	rpcCli, err := rpc.DialContext(context.Background(), opt.rpcURL)
	if err != nil {
		log.Error("connect eth rpc", "url", opt.rpcURL, "err", err)
		os.Exit(1)
	}
	defer rpcCli.Close()

	p, err := newProbe(rpcCli)
	if err != nil {
		log.Error("parse ERC20 ABI", "err", err)
		os.Exit(1)
	}

	log.Info("start", "stage", opt.stage, "rpc", opt.rpcURL, "dsn", logger.MaskDSN(opt.dsn),
		"dryRun", opt.dryRun, "concurrency", opt.concurrency, "limit", opt.limit)

	if opt.stage == "contracts" || opt.stage == "all" {
		if err := runContracts(context.Background(), log, db, p, opt); err != nil {
			log.Error("contracts stage failed", "err", err)
			os.Exit(1)
		}
	}
	if opt.stage == "balances" || opt.stage == "all" {
		if err := runBalances(context.Background(), log, db, p, opt); err != nil {
			log.Error("balances stage failed", "err", err)
			os.Exit(1)
		}
	}
	if opt.stage == "backfill-value" {
		if err := runBackfillValue(context.Background(), log, db, p, opt); err != nil {
			log.Error("backfill-value stage failed", "err", err)
			os.Exit(1)
		}
	}
	if opt.stage == "prune-eoa" {
		if err := runPruneEOA(context.Background(), log, db, p, opt); err != nil {
			log.Error("prune-eoa stage failed", "err", err)
			os.Exit(1)
		}
	}
	log.Info("done")
}

// missing 返回缺少的参数名，用于错误提示
func missing(opt options) []string {
	var out []string
	if opt.dsn == "" {
		out = append(out, "-dsn 或 -c 里的 [mysql].dsn")
	}
	if opt.rpcURL == "" {
		out = append(out, "-rpc 或 -c 里的 [erc20scan].ethUrl")
	}
	return out
}

// ---------------- 合约判定 ----------------

type probe struct {
	rpc *rpc.Client
	abi abi.ABI
}

func newProbe(cli *rpc.Client) (*probe, error) {
	parsed, err := abi.JSON(strings.NewReader(generated.ERC20ABI))
	if err != nil {
		return nil, err
	}
	return &probe{rpc: cli, abi: parsed}, nil
}

// codeAt 取合约代码。
// chain33 的 eth_getCode 对"没有代码的地址"返回 null（GetCode 里 ret.Code 为 nil），
// 而 geth 的 hexutil.Bytes 会把 null 当成类型错误（"cannot unmarshal non-string"），
// 所以这里用 RawMessage 自己解：null / "" / "0x" 都当"没有代码"。
func (p *probe) codeAt(ctx context.Context, addr common.Address) ([]byte, error) {
	var raw json.RawMessage
	if err := p.rpc.CallContext(ctx, &raw, "eth_getCode", addr, "latest"); err != nil {
		return nil, err
	}
	var s *string
	if err := json.Unmarshal(raw, &s); err != nil {
		// 非字符串（对象/数字等）说明节点行为异常，当错误处理
		return nil, fmt.Errorf("eth_getCode 返回非字符串结果: %s", strings.TrimSpace(string(raw)))
	}
	if s == nil || *s == "" || *s == "0x" {
		return nil, nil
	}
	code, err := hexutil.Decode(*s)
	if err != nil {
		return nil, fmt.Errorf("eth_getCode 解码失败: %w", err)
	}
	return code, nil
}

// call 发 eth_call：calldata 同时放 data 和 input（chain33 只认 data，geth 1.14 只发 input）。
func (p *probe) call(ctx context.Context, to common.Address, method string, args ...interface{}) ([]interface{}, error) {
	data, err := p.abi.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	arg := map[string]interface{}{
		"to":    to,
		"data":  hexutil.Bytes(data),
		"input": hexutil.Bytes(data),
	}
	var raw json.RawMessage
	if err := p.rpc.CallContext(ctx, &raw, "eth_call", arg, "latest"); err != nil {
		return nil, err
	}
	var out hexutil.Bytes
	if err := json.Unmarshal(raw, &out); err != nil {
		// chain33 在合约 revert 后可能直接回 null，这里统一当"空返回"处理，
		// 交给 classifyErr 归到 empty-return（确定性错误，不重试、不写库）。
		if s := strings.TrimSpace(string(raw)); s == "null" || s == "" {
			return nil, fmt.Errorf("%s: empty return data", method)
		}
		return nil, fmt.Errorf("%s: 返回非字符串结果: %s", method, strings.TrimSpace(string(raw)))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: empty return data", method)
	}
	vals, err := p.abi.Unpack(method, out)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	return vals, nil
}

// probeResult 判定结果；HasCode 用来区分"不是合约"和"是合约但不是 ERC20"
type probeResult struct {
	IsERC20   bool
	HasCode   bool
	CoreFound int
	Err       error
}

// check 与 scanner 口径一致：有代码 + "63"+selector 至少 4 个 + totalSupply 能调通
func (p *probe) check(ctx context.Context, addr common.Address) probeResult {
	code, err := p.codeAt(ctx, addr)
	if err != nil {
		return probeResult{Err: fmt.Errorf("eth_getCode: %w", err)}
	}
	if len(code) == 0 {
		return probeResult{}
	}
	codeHex := hex.EncodeToString(code)
	found := 0
	for _, s := range coreSelectors {
		if strings.Contains(codeHex, "63"+s) {
			found++
		}
	}
	res := probeResult{HasCode: true, CoreFound: found}
	if found < 4 {
		return res
	}
	if _, err := p.call(ctx, addr, "totalSupply"); err != nil {
		res.Err = fmt.Errorf("totalSupply: %w", err)
		return res
	}
	res.IsERC20 = true
	return res
}

// tokenMeta 读 name/symbol/decimals/totalSupply；name/symbol/decimals 失败不致命
func (p *probe) tokenMeta(ctx context.Context, addr common.Address) (string, string, uint8, *big.Int) {
	var name, symbol string
	var decimals uint8 = 18
	var supply *big.Int

	if vals, err := p.call(ctx, addr, "name"); err == nil && len(vals) > 0 {
		if s, ok := vals[0].(string); ok {
			name = s
		}
	}
	if vals, err := p.call(ctx, addr, "symbol"); err == nil && len(vals) > 0 {
		if s, ok := vals[0].(string); ok {
			symbol = s
		}
	}
	if vals, err := p.call(ctx, addr, "decimals"); err == nil && len(vals) > 0 {
		if d, ok := vals[0].(uint8); ok {
			decimals = d
		}
	}
	if vals, err := p.call(ctx, addr, "totalSupply"); err == nil && len(vals) > 0 {
		if v, ok := vals[0].(*big.Int); ok {
			supply = v
		}
	}
	return name, symbol, decimals, supply
}

// txValueByHash 用 eth_getTransactionByHash 取主币金额（wei）
func (p *probe) txValueByHash(ctx context.Context, hash string) (*big.Int, error) {
	var raw json.RawMessage
	if err := p.rpc.CallContext(ctx, &raw, "eth_getTransactionByHash", hash); err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, fmt.Errorf("节点查不到该交易（可能已被裁剪）")
	}
	var tx struct {
		Value *hexutil.Big `json:"value"`
	}
	if err := json.Unmarshal(raw, &tx); err != nil {
		return nil, err
	}
	if tx.Value == nil {
		return nil, fmt.Errorf("返回里没有 value 字段")
	}
	return (*big.Int)(tx.Value), nil
}

func (p *probe) balanceOf(ctx context.Context, contract, holder common.Address) (*big.Int, error) {
	vals, err := p.call(ctx, contract, "balanceOf", holder)
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("balanceOf: 空结果")
	}
	v, ok := vals[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("balanceOf: 结果类型 %T", vals[0])
	}
	return v, nil
}

// clampAmount MySQL DECIMAL(65,0) 装不下时按线上口径记 -1（表示 unlimited）
func clampAmount(v *big.Int) string {
	if v == nil {
		return "0"
	}
	if v.Cmp(maxDecimal65) > 0 {
		return "-1"
	}
	return v.String()
}

type contractStats struct {
	updated   int64 // 判成 ERC20 并写库（dry-run 时是"将写"）
	notERC20  int64 // 有代码但选择器不够
	noCode    int64 // 地址没代码（不是合约）
	callErr   int64 // eth_call 失败（不写终态，下次可重跑）
	processed int64
	errKinds  map[string]int64 // 失败原因分类计数
}

// classifyErr 把错误归类，便于判断"合约本身不行"还是"节点限流/抖动"
func classifyErr(err error) string {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "empty return data"):
		return "empty-return(合约返回0x，通常是 fallback revert)"
	case strings.Contains(s, "execution reverted"):
		return "reverted(合约 revert)"
	case strings.Contains(s, "deadline exceeded"), strings.Contains(s, "timeout"), strings.Contains(s, "timed out"):
		return "timeout(超时，可能是并发太高/节点慢)"
	case strings.Contains(s, "429"), strings.Contains(s, "too many"), strings.Contains(s, "rate"):
		return "rate-limit(节点限流)"
	case strings.Contains(s, "connection"), strings.Contains(s, "eof"), strings.Contains(s, "reset"), strings.Contains(s, "refused"):
		return "network(连接被断)"
	default:
		return "other(" + s + ")"
	}
}

// transient 判断是否值得重试（合约语义类错误重试没用）
func transient(kind string) bool {
	return !strings.HasPrefix(kind, "empty-return") && !strings.HasPrefix(kind, "reverted")
}

// checkWithRetry 对瞬时错误重试；确定性错误直接返回
func (p *probe) checkWithRetry(ctx context.Context, addr common.Address, retries int) probeResult {
	res := p.check(ctx, addr)
	for i := 0; i < retries && res.Err != nil; i++ {
		if !transient(classifyErr(res.Err)) {
			break
		}
		select {
		case <-ctx.Done():
			return res
		case <-time.After(time.Duration(200*(i+1)) * time.Millisecond):
		}
		res = p.check(ctx, addr)
	}
	return res
}

func runContracts(ctx context.Context, log *slog.Logger, db *sql.DB, p *probe, opt options) error {
	query := `SELECT contract_address FROM contracts WHERE contract_type='UNKNOWN' ORDER BY id`
	if !opt.onlyUnknown {
		query = `SELECT contract_address FROM contracts ORDER BY id`
	}
	if opt.limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opt.limit)
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query unknown contracts: %w", err)
	}
	addrs := make([]string, 0, 1024)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return err
		}
		addrs = append(addrs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	log.Info("contracts stage", "candidates", len(addrs), "onlyUnknown", opt.onlyUnknown)
	if len(addrs) == 0 {
		return nil
	}

	st := contractStats{errKinds: map[string]int64{}}
	var mu sync.Mutex
	var samples []string

	updSQL := `UPDATE contracts SET contract_type='ERC20', contract_name=?, contract_symbol=?, decimals=?, total_supply=?,
		verification_status=1, verified_functions=? WHERE contract_address=?`

	jobs := make(chan string, opt.concurrency*2)
	var wg sync.WaitGroup
	fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for raw := range jobs {
				addr := common.HexToAddress(strings.ToLower(raw))
				cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
				res := p.checkWithRetry(cctx, addr, opt.retries)
				atomic.AddInt64(&st.processed, 1)
				switch {
				case res.Err != nil:
					atomic.AddInt64(&st.callErr, 1)
					kind := classifyErr(res.Err)
					mu.Lock()
					st.errKinds[kind]++
					mu.Unlock()
					log.Warn("probe failed", "contract", raw, "kind", kind, "err", res.Err)
				case !res.HasCode:
					atomic.AddInt64(&st.noCode, 1)
					if opt.verbose {
						log.Info("not a contract", "contract", raw)
					}
				case !res.IsERC20:
					atomic.AddInt64(&st.notERC20, 1)
					if opt.verbose {
						log.Info("not erc20", "contract", raw, "coreSelectors", res.CoreFound)
					}
				default:
					name, symbol, decimals, supply := p.tokenMeta(cctx, addr)
					atomic.AddInt64(&st.updated, 1)
					if opt.dryRun {
						mu.Lock()
						if len(samples) < 5 {
							samples = append(samples, fmt.Sprintf(
								"UPDATE contracts SET contract_type='ERC20', contract_name='%s', contract_symbol='%s', decimals=%d, total_supply=%s WHERE contract_address='%s';",
								name, symbol, decimals, clampAmount(supply), strings.ToLower(raw)))
						}
						mu.Unlock()
						log.Info("would update", "contract", raw, "name", name, "symbol", symbol,
							"decimals", decimals, "totalSupply", clampAmount(supply))
					} else {
						if _, err := db.ExecContext(cctx, updSQL, name, symbol, decimals, clampAmount(supply),
							verifiedFuncsJSON, strings.ToLower(raw)); err != nil {
							atomic.AddInt64(&st.callErr, 1)
							log.Error("update contract failed", "contract", raw, "err", err)
						}
					}
				}
				ccancel()
			}
		}()
	}

	for _, a := range addrs {
		select {
		case jobs <- a:
		case <-fetchCtx.Done():
			log.Warn("load addresses timeout, stop feeding")
		}
	}
	close(jobs)
	wg.Wait()

	if opt.dryRun && len(samples) > 0 {
		fmt.Println("\n=== 样例 SQL（dry-run） ===")
		for _, s := range samples {
			fmt.Println(s)
		}
	}
	log.Info("contracts stage done",
		"processed", st.processed, "updated", st.updated, "notERC20", st.notERC20,
		"noCode", st.noCode, "callErr", st.callErr, "dryRun", opt.dryRun)
	for kind, n := range st.errKinds {
		log.Info("eth_call 失败原因", "kind", kind, "count", n)
	}
	if st.callErr > 0 {
		log.Warn("有 eth_call 失败的地址未写库，可直接重跑（幂等）", "count", st.callErr)
	}
	return nil
}

// ---------------- 余额重建 ----------------

type pair struct {
	contract string
	holder   string
}

type balanceStats struct {
	written int64
	zero    int64
	failed  int64
	skipped int64
}

// loadPairs 从 events 里取 (合约, 持有人) 去重对：Transfer 的 from/to 都算，排除零地址
func loadPairs(ctx context.Context, log *slog.Logger, db *sql.DB) ([]pair, error) {
	q := `SELECT contract_address, holder FROM (
			SELECT contract_address, from_address AS holder FROM events
			WHERE event_name='Transfer' AND from_address <> ? AND from_address IS NOT NULL
			UNION
			SELECT contract_address, to_address AS holder FROM events
			WHERE event_name='Transfer' AND to_address <> ? AND to_address IS NOT NULL
		) t`
	start := time.Now()
	rows, err := db.QueryContext(ctx, q, zeroAddr, zeroAddr)
	if err != nil {
		return nil, fmt.Errorf("query pairs: %w", err)
	}
	defer rows.Close()
	out := make([]pair, 0, 200000)
	for rows.Next() {
		var pr pair
		if err := rows.Scan(&pr.contract, &pr.holder); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	log.Info("pairs loaded", "pairs", len(out), "cost", time.Since(start).Round(time.Second))
	return out, nil
}

func loadERC20Set(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT contract_address FROM contracts WHERE contract_type='ERC20'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool, 4096)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		set[strings.ToLower(a)] = true
	}
	return set, rows.Err()
}

func runBalances(ctx context.Context, log *slog.Logger, db *sql.DB, p *probe, opt options) error {
	erc20Set, err := loadERC20Set(ctx, db)
	if err != nil {
		return fmt.Errorf("load erc20 contracts: %w", err)
	}
	log.Info("balances stage", "erc20Contracts", len(erc20Set))
	if len(erc20Set) == 0 {
		log.Warn("contracts 表里没有 ERC20，先跑 -stage contracts")
		return nil
	}

	pairs, err := loadPairs(ctx, log, db)
	if err != nil {
		return err
	}

	filtered := make([]pair, 0, len(pairs))
	skipped := 0
	for _, pr := range pairs {
		if !erc20Set[strings.ToLower(pr.contract)] {
			skipped++
			continue
		}
		filtered = append(filtered, pr)
	}
	log.Info("pairs filtered", "candidates", len(filtered), "skippedNonERC20", skipped)
	if opt.pairsLimit > 0 && len(filtered) > opt.pairsLimit {
		filtered = filtered[:opt.pairsLimit]
		log.Info("pairs limited", "pairs", len(filtered))
	}

	upSQL := `INSERT INTO token_balances (address, contract_address, balance, last_tx_hash, last_tx_block_number)
		VALUES (?, ?, ?, NULL, NULL)
		ON DUPLICATE KEY UPDATE balance=VALUES(balance), last_updated_at=CURRENT_TIMESTAMP`

	var st balanceStats
	atomic.AddInt64(&st.skipped, int64(skipped))

	jobs := make(chan pair, opt.concurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pr := range jobs {
				holder := strings.ToLower(pr.holder)
				contract := strings.ToLower(pr.contract)
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				bal, err := p.balanceOf(cctx, common.HexToAddress(contract), common.HexToAddress(holder))
				if err != nil {
					atomic.AddInt64(&st.failed, 1)
					log.Warn("balanceOf failed", "contract", contract, "holder", holder, "err", err)
					cancel()
					continue
				}
				if bal.Sign() == 0 {
					atomic.AddInt64(&st.zero, 1)
				}
				if opt.dryRun {
					atomic.AddInt64(&st.written, 1)
				} else if _, err := db.ExecContext(cctx, upSQL, holder, contract, clampAmount(bal)); err != nil {
					atomic.AddInt64(&st.failed, 1)
					log.Error("upsert balance failed", "contract", contract, "holder", holder, "err", err)
				} else {
					atomic.AddInt64(&st.written, 1)
				}
				cancel()
			}
		}()
	}
	for _, pr := range filtered {
		jobs <- pr
	}
	close(jobs)
	wg.Wait()

	log.Info("balances stage done", "written", st.written, "zeroBalance", st.zero,
		"failed", st.failed, "skippedNonERC20", st.skipped, "dryRun", opt.dryRun)
	return nil
}

// ---------------- 清理 EOA 占位行 ----------------
//
// scanner 旧版本把"普通 ETH 转账的 to"也当合约地址写进 contracts（EnsureContract 满足外键），
// 线上留下 5.8 万条没有代码的 UNKNOWN 行。新版本已改为统一指向 database.NativeTransferAddress，
// 本阶段用于清理历史：只处理**链上无代码**且**没有被 events/token_balances/token_allowances 引用**的行，
// 先把 transactions.contract_address 改到主币哨兵地址，再删 contracts 行（外键 RESTRICT，顺序不能反）。

const upsertNativeContractSQL = `INSERT INTO contracts (
		contract_address, contract_name, contract_symbol, contract_type, decimals, verification_status, verified_functions
	) VALUES (?, 'Native', 'BTY', 'NATIVE', 18, 0, '[]')
	ON DUPLICATE KEY UPDATE contract_name=VALUES(contract_name), contract_symbol=VALUES(contract_symbol),
		contract_type='NATIVE', decimals=18`

type pruneStats struct {
	examined   int64
	pruned     int64
	hasCode    int64
	referenced int64
	codeErr    int64
	updatedTx  int64
}

func runPruneEOA(ctx context.Context, log *slog.Logger, db *sql.DB, p *probe, opt options) error {
	log.Info("prune-eoa stage", "nativeAddr", database.NativeTransferAddress, "dryRun", opt.dryRun)
	if opt.dryRun {
		log.Info("would ensure native contract row", "address", database.NativeTransferAddress)
	} else if _, err := db.ExecContext(ctx, upsertNativeContractSQL, database.NativeTransferAddress); err != nil {
		return fmt.Errorf("ensure native contract row: %w", err)
	}

	query := `SELECT contract_address FROM contracts WHERE contract_type='UNKNOWN' ORDER BY id`
	if opt.limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opt.limit)
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query unknown contracts: %w", err)
	}
	addrs := make([]string, 0, 1024)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return err
		}
		addrs = append(addrs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	log.Info("prune-eoa candidates", "unknown", len(addrs))

	var st pruneStats
	var mu sync.Mutex
	jobs := make(chan string, opt.concurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for raw := range jobs {
				addr := strings.ToLower(raw)
				atomic.AddInt64(&st.examined, 1)
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				code, err := p.codeAt(cctx, common.HexToAddress(addr))
				cancel()
				switch {
				case err != nil:
					atomic.AddInt64(&st.codeErr, 1)
					log.Warn("eth_getCode failed, skip", "contract", addr, "err", err)
					continue
				case len(code) > 0:
					atomic.AddInt64(&st.hasCode, 1) // 真合约，不动
					continue
				}

				cctx, cancel = context.WithTimeout(ctx, 30*time.Second)
				refs, err := countRefs(cctx, db, addr)
				cancel()
				if err != nil {
					atomic.AddInt64(&st.codeErr, 1)
					log.Warn("count refs failed, skip", "contract", addr, "err", err)
					continue
				}
				if refs > 0 {
					atomic.AddInt64(&st.referenced, 1)
					continue
				}

				if opt.dryRun {
					atomic.AddInt64(&st.pruned, 1)
					mu.Lock()
					if st.pruned <= 10 {
						log.Info("would prune", "contract", addr)
					}
					mu.Unlock()
					continue
				}
				n, err := pruneOne(ctx, db, addr)
				if err != nil {
					atomic.AddInt64(&st.codeErr, 1)
					log.Error("prune failed", "contract", addr, "err", err)
					continue
				}
				atomic.AddInt64(&st.pruned, 1)
				atomic.AddInt64(&st.updatedTx, n)
			}
		}()
	}
	for _, a := range addrs {
		jobs <- a
	}
	close(jobs)
	wg.Wait()

	log.Info("prune-eoa stage done",
		"examined", st.examined, "pruned", st.pruned, "txMovedToNative", st.updatedTx,
		"keptHasCode", st.hasCode, "keptReferenced", st.referenced, "errors", st.codeErr, "dryRun", opt.dryRun)
	return nil
}

// countRefs 该地址是否被事件/余额/授权引用（被引用就不能删 contracts 行）
func countRefs(ctx context.Context, db *sql.DB, addr string) (int64, error) {
	var n int64
	q := `SELECT
		(SELECT COUNT(*) FROM events e WHERE e.contract_address = ?) +
		(SELECT COUNT(*) FROM token_balances b WHERE b.contract_address = ?) +
		(SELECT COUNT(*) FROM token_allowances a WHERE a.contract_address = ?)`
	if err := db.QueryRowContext(ctx, q, addr, addr, addr).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// pruneOne 把一个 EOA 占位行的交易改指主币哨兵地址后删除该 contracts 行（单事务）。
// 线上 scanner 同时在写，偶发 Error 1205(Lock wait timeout)/1213(Deadlock)，退避重试即可。
func pruneOne(ctx context.Context, db *sql.DB, addr string) (int64, error) {
	const maxTry = 5
	var lastErr error
	for try := 1; try <= maxTry; try++ {
		n, err := pruneOneOnce(ctx, db, addr)
		if err == nil {
			return n, nil
		}
		if !isTransientDBErr(err) {
			return 0, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(try) * 300 * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("重试 %d 次仍失败: %w", maxTry, lastErr)
}

func isLockErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "1205") || strings.Contains(s, "1213") ||
		strings.Contains(s, "Lock wait timeout") || strings.Contains(s, "Deadlock")
}

// isTransientDBErr 可重试的 DB 错误：锁冲突 + 连接被服务端/代理断掉。
// "invalid connection" 是 go-sql-driver 拿到已关闭连接时的错误，
// 重试会从池里取一条新连接；对应 UPDATE 幂等，重试安全。
func isTransientDBErr(err error) bool {
	if err == nil {
		return false
	}
	if isLockErr(err) {
		return true
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, kw := range []string{"invalid connection", "unexpected eof", "bad connection",
		"connection reset", "broken pipe", "i/o timeout", "busy buffer", "server has gone away",
		"connection refused"} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// pruneChunk 单个地址每批搬多少条交易。一个 EOA 名下可能有几十万笔交易，
// 一次性 UPDATE 会锁住全部行（实测把并发写方拖到 Error 1205），所以分批提交。
const pruneChunk = 5000

func pruneOneOnce(ctx context.Context, db *sql.DB, addr string) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx,
			`UPDATE transactions SET contract_address=? WHERE contract_address=? LIMIT ?`,
			database.NativeTransferAddress, addr, pruneChunk)
		if err != nil {
			if isTransientDBErr(err) {
				return total, err // 交给外层退避重试
			}
			return total, fmt.Errorf("update transactions: %w", err)
		}
		n, _ := res.RowsAffected()
		total += n
		if n < pruneChunk {
			break
		}
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM contracts WHERE contract_address=? AND contract_type='UNKNOWN'`, addr); err != nil {
		if isTransientDBErr(err) {
			return total, err
		}
		return total, fmt.Errorf("delete contract: %w", err)
	}
	return total, nil
}

// ---------------- 回填 transactions.value ----------------
//
// 修复前（d35df9c 之前）普通主币转账的 value 恒为 NULL，历史行只能回链上按 tx hash 取。
// 用游标分批（id > cursor ORDER BY id LIMIT batch）：value 没有索引，若每轮都从头扫，
// 已补过的行会重复扫到，成本会随进度线性上涨（几百万行后一轮只剩几十条的产出）。
func runBackfillValue(ctx context.Context, log *slog.Logger, db *sql.DB, p *probe, opt options) error {
	base := `SELECT id, tx_hash FROM transactions WHERE value IS NULL`
	args := []interface{}{}
	if strings.ToLower(opt.funcs) != "all" {
		var ph []string
		for _, f := range strings.Split(opt.funcs, ",") {
			if f = strings.TrimSpace(f); f != "" {
				ph = append(ph, "?")
				args = append(args, f)
			}
		}
		if len(ph) > 0 {
			base += " AND func_name IN (" + strings.Join(ph, ",") + ")"
		}
	}
	if opt.fromBlock > 0 {
		base += " AND block_number >= ?"
		args = append(args, opt.fromBlock)
	}

	cursor := opt.afterID
	var scanned, updated, missing, failed int64
	started := time.Now()
	batch := opt.batchSize
	if batch <= 0 {
		batch = 2000
	}

	for {
		if opt.limit > 0 && updated+missing+failed >= int64(opt.limit) {
			log.Info("达到 -limit，停止（可用 -after-id 续跑）", "limit", opt.limit, "afterID", cursor)
			break
		}
		pageArgs := append(append([]interface{}{}, args...), cursor, batch)
		rows, err := db.QueryContext(ctx, base+" AND id > ? ORDER BY id LIMIT ?", pageArgs...)
		if err != nil {
			return fmt.Errorf("query null-value transactions: %w", err)
		}
		type item struct {
			id   uint64
			hash string
		}
		items := make([]item, 0, batch)
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.id, &it.hash); err != nil {
				rows.Close()
				return err
			}
			items = append(items, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) == 0 {
			log.Info("没有更多待补行", "afterID", cursor)
			break
		}
		cursor = int64(items[len(items)-1].id)
		scanned += int64(len(items))

		if opt.dryRun {
			updated += int64(len(items))
			log.Info("dry-run 页", "rows", len(items), "afterID", cursor)
			continue
		}

		jobs := make(chan item, opt.concurrency*2)
		var wg sync.WaitGroup
		for i := 0; i < opt.concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for it := range jobs {
					var val *big.Int
					var getErr error
					for try := 0; try <= opt.retries; try++ {
						cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
						val, getErr = p.txValueByHash(cctx, it.hash)
						cancel()
						if getErr == nil || !transient(classifyErr(getErr)) {
							break
						}
						time.Sleep(time.Duration(200*(try+1)) * time.Millisecond)
					}
					if getErr != nil {
						if strings.Contains(getErr.Error(), "查不到") || strings.Contains(getErr.Error(), "没有 value") {
							atomic.AddInt64(&missing, 1)
						} else {
							atomic.AddInt64(&failed, 1)
							log.Warn("get tx value failed", "tx", it.hash, "err", getErr)
						}
						continue
					}
					var dbErr error
					for try := 0; try < 5; try++ {
						_, dbErr = db.ExecContext(ctx,
							`UPDATE transactions SET value=? WHERE id=? AND value IS NULL`, clampAmount(val), it.id)
						if dbErr == nil || !isTransientDBErr(dbErr) {
							break
						}
						select {
						case <-ctx.Done():
						case <-time.After(time.Duration(try+1) * 300 * time.Millisecond):
						}
					}
					if dbErr != nil {
						atomic.AddInt64(&failed, 1)
						log.Error("update value failed", "tx", it.hash, "err", dbErr)
						continue
					}
					atomic.AddInt64(&updated, 1)
				}
			}()
		}
		for _, it := range items {
			jobs <- it
		}
		close(jobs)
		wg.Wait()

		if scanned%(int64(batch)*10) == 0 {
			el := time.Since(started).Seconds()
			log.Info("进度", "scanned", scanned, "updated", updated, "notFound", missing,
				"failed", failed, "rows/s", int(float64(updated)/el), "afterID", cursor)
		}
	}

	log.Info("backfill-value stage done", "scanned", scanned, "updated", updated,
		"notFound", missing, "failed", failed, "afterID", cursor,
		"cost", time.Since(started).Round(time.Second), "dryRun", opt.dryRun)
	if failed > 0 {
		log.Warn("有失败行未补，重跑即可（幂等）", "failed", failed)
	}
	return nil
}
