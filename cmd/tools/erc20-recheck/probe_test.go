package main

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/33cn/externaldb/erc20/erc20abi/generated"
)

// mockRPC 模拟 chain33 的 eth JSON-RPC：只认 data 字段（chain33 v1.72 的 CallMsg 没有 input），
// 从而保证工具真的把 calldata 写在 data 里。
func mockRPC(t *testing.T, codeByAddr map[string]string, emptyCallFor map[string]bool) (*rpc.Client, *[]map[string]interface{}) {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(generated.ERC20ABI))
	if err != nil {
		t.Fatalf("parse erc20 abi: %v", err)
	}
	returns := map[string]interface{}{
		"totalSupply": big.NewInt(1_000_000),
		"name":        "Test Token",
		"symbol":      "TT",
		"decimals":    uint8(6),
		"balanceOf":   big.NewInt(42),
	}

	var seen []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("bad request %s: %v", body, err)
			return
		}
		var result interface{}
		switch req.Method {
		case "eth_getCode":
			var addr string
			_ = json.Unmarshal(req.Params[0], &addr)
			if v, ok := codeByAddr[strings.ToLower(addr)]; ok {
				if v == "NULL" { // 模拟 chain33 对没代码的地址返回 null
					result = nil
				} else {
					result = v
				}
			} else {
				result = "0x"
			}
		case "eth_call":
			var arg map[string]json.RawMessage
			_ = json.Unmarshal(req.Params[0], &arg)
			var to string
			_ = json.Unmarshal(arg["to"], &to)
			if emptyCallFor[strings.ToLower(to)] {
				result = nil // 模拟 chain33：revert 后直接回 null
				break
			}
			seen = append(seen, map[string]interface{}{"hasData": arg["data"] != nil, "hasInput": arg["input"] != nil})
			if arg["data"] == nil {
				t.Errorf("eth_call 缺少 data 字段（chain33 会丢弃 calldata）: %s", req.Params[0])
				return
			}
			var data string
			_ = json.Unmarshal(arg["data"], &data)
			if len(data) < 10 {
				result = "0x"
				break
			}
			sel := data[2:10]
			var method string
			for name, m := range parsed.Methods {
				if hexutil.Encode(m.ID)[2:10] == sel {
					method = name
				}
			}
			if method == "" {
				result = "0x"
				break
			}
			out, err := parsed.Methods[method].Outputs.Pack(returns[method])
			if err != nil {
				t.Errorf("pack %s: %v", method, err)
				return
			}
			result = hexutil.Encode(out)
		default:
			t.Errorf("unexpected method %s", req.Method)
			return
		}
		resp, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	cli, err := rpc.DialContext(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("dial mock rpc: %v", err)
	}
	t.Cleanup(cli.Close)
	return cli, &seen
}

func erc20CodeHex() string {
	var b strings.Builder
	b.WriteString("0x6080604052")
	for _, s := range coreSelectors {
		b.WriteString("63")
		b.WriteString(s)
	}
	return b.String()
}

func TestProbeIsERC20(t *testing.T) {
	good := "0x00000000000000000000000000000000000000a1"
	junk := "0x00000000000000000000000000000000000000a2"
	noCode := "0x00000000000000000000000000000000000000a3"

	cli, _ := mockRPC(t, map[string]string{
		good: erc20CodeHex(),
		junk: "0x6080604052600080fd", // 没有任何 63+selector
	}, nil)
	p, err := newProbe(cli)
	if err != nil {
		t.Fatalf("newProbe: %v", err)
	}
	ctx := context.Background()

	if r := p.check(ctx, common.HexToAddress(good)); r.Err != nil || !r.IsERC20 || !r.HasCode || r.CoreFound != 6 {
		t.Fatalf("good: %+v，want IsERC20/HasCode/CoreFound=6", r)
	}
	if r := p.check(ctx, common.HexToAddress(junk)); r.Err != nil || r.IsERC20 || !r.HasCode || r.CoreFound != 0 {
		t.Fatalf("junk: %+v，want 有代码但不是 ERC20", r)
	}
	if r := p.check(ctx, common.HexToAddress(noCode)); r.Err != nil || r.IsERC20 || r.HasCode || r.CoreFound != 0 {
		t.Fatalf("noCode: %+v，want 没代码", r)
	}

	name, symbol, decimals, supply := p.tokenMeta(ctx, common.HexToAddress(good))
	if name != "Test Token" || symbol != "TT" || decimals != 6 || supply.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("tokenMeta = %q/%q/%d/%s", name, symbol, decimals, supply)
	}

	bal, err := p.balanceOf(ctx, common.HexToAddress(good), common.HexToAddress("0x00000000000000000000000000000000000000b1"))
	if err != nil || bal.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("balanceOf = %v, err=%v", bal, err)
	}
}

// eth_call 返回空（事故现场的表现）时，isERC20 必须返回 error 而不是 false ——
// 工具据此"不写终态"，避免又造出一批永久冻结的 UNKNOWN。
func TestProbeEmptyCallIsError(t *testing.T) {
	addr := "0x00000000000000000000000000000000000000c1"
	cli, _ := mockRPC(t, map[string]string{addr: erc20CodeHex()}, map[string]bool{addr: true})
	p, err := newProbe(cli)
	if err != nil {
		t.Fatalf("newProbe: %v", err)
	}
	r := p.check(context.Background(), common.HexToAddress(addr))
	if r.Err == nil {
		t.Fatalf("空返回应报错，实际 %+v", r)
	}
	if r.IsERC20 {
		t.Fatalf("空返回不能判成 ERC20")
	}
}

// chain33 对没有代码的地址返回 null，geth 的 hexutil.Bytes 解析 null 会报类型错误；
// 工具必须把它当"没有代码"（noCode），而不是 callErr。
func TestProbeNullCodeMeansNoContract(t *testing.T) {
	eoa := "0x00000000000000000000000000000000000000d1"
	cli, _ := mockRPC(t, map[string]string{eoa: "NULL"}, nil)
	p, err := newProbe(cli)
	if err != nil {
		t.Fatalf("newProbe: %v", err)
	}
	r := p.check(context.Background(), common.HexToAddress(eoa))
	if r.Err != nil {
		t.Fatalf("null 代码不应报错: %v", r.Err)
	}
	if r.HasCode || r.IsERC20 {
		t.Fatalf("null 代码应判为无代码: %+v", r)
	}
}

func TestClampAmount(t *testing.T) {
	if got := clampAmount(nil); got != "0" {
		t.Fatalf("clampAmount(nil) = %q", got)
	}
	if got := clampAmount(big.NewInt(123)); got != "123" {
		t.Fatalf("clampAmount(123) = %q", got)
	}
	// 2^256-1 超出 DECIMAL(65,0) 范围 → -1
	huge := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if got := clampAmount(huge); got != "-1" {
		t.Fatalf("clampAmount(2^256-1) = %q", got)
	}
}

func TestClassifyErr(t *testing.T) {
	cases := []struct {
		err  string
		want string
	}{
		{"totalSupply: empty return data", "empty-return"},
		{"eth_call: execution reverted: x", "reverted"},
		{"Post \"http://x\": context deadline exceeded", "timeout"},
		{"429 Too Many Requests", "rate-limit"},
		{"read tcp 1.2.3.4: connection reset by peer", "network"},
		{"some weird failure", "other"},
	}
	for _, c := range cases {
		got := classifyErr(errors.New(c.err))
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("classifyErr(%q) = %q, want prefix %q", c.err, got, c.want)
		}
	}
	if transient("empty-return(x)") || transient("reverted(x)") {
		t.Error("确定性错误不该重试")
	}
	if !transient("timeout(x)") || !transient("rate-limit(x)") {
		t.Error("瞬时错误应该重试")
	}
}

func TestIsTransientDBErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("Error 1205 (HY000): Lock wait timeout exceeded"), true},
		{errors.New("Error 1213 (40001): Deadlock found"), true},
		{errors.New("invalid connection"), true},
		{errors.New("[mysql] packets.go:58 unexpected EOF"), true},
		{errors.New("write tcp: broken pipe"), true},
		{driver.ErrBadConn, true},
		{errors.New("Error 1062 (23000): Duplicate entry"), false},
		{errors.New("Error 1146 (42S02): Table doesn't exist"), false},
		{nil, false},
	}
	for _, c := range cases {
		if got := isTransientDBErr(c.err); got != c.want {
			t.Errorf("isTransientDBErr(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
