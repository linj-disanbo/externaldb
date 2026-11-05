package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// 回归：eth_call 必须同时带 data 和 input。
// 背景：go-ethereum v1.14 的 ethclient 只发 input，而 chain33 v1.72 的
// rpc/ethrpc/types.CallMsg 只有 json:"data"，只发 input 会被丢弃 → 以空 calldata 执行 →
// 合约 fallback revert → 节点返回空结果 → "abi: attempting to unmarshal an empty string"，
// scanner 会因此把全部合约判成 UNKNOWN（并且 balanceOf 也一起失效）。
func TestCallContractSendsDataAndInput(t *testing.T) {
	const wantResult = "0x0000000000000000000000000000000000000000000000000000000000000005"

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"` + wantResult + `"}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	rpcCli, err := rpc.DialContext(ctx, srv.URL)
	if err != nil {
		t.Fatalf("dial mock rpc: %v", err)
	}
	defer rpcCli.Close()

	c := &Client{rpcCli: rpcCli, cli: ethclient.NewClient(rpcCli)}
	to := common.HexToAddress("0xE09f5BdCA143f6e4Ad9d43516A1D1289A3DD6dFC")
	// totalSupply() 的 selector
	data := []byte{0x18, 0x16, 0x0d, 0xdd}

	got, err := c.CallContract(ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		t.Fatalf("CallContract: %v", err)
	}
	if hexutil.Encode(got) != wantResult {
		t.Fatalf("result = %s, want %s", hexutil.Encode(got), wantResult)
	}

	var req struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request body %s: %v", body, err)
	}
	if req.Method != "eth_call" {
		t.Fatalf("method = %q, want eth_call", req.Method)
	}
	if len(req.Params) != 2 {
		t.Fatalf("params 个数 = %d, want 2（callArg + blockTag）: %s", len(req.Params), body)
	}
	if string(req.Params[1]) != `"latest"` {
		t.Fatalf("block tag = %s, want \"latest\"", req.Params[1])
	}

	var callArg map[string]json.RawMessage
	if err := json.Unmarshal(req.Params[0], &callArg); err != nil {
		t.Fatalf("decode call arg %s: %v", req.Params[0], err)
	}
	rawData, rawInput := callArg["data"], callArg["input"]
	if rawData == nil {
		t.Fatalf("eth_call 缺少 data 字段（chain33 v1.72 只认 data）: %s", req.Params[0])
	}
	if rawInput == nil {
		t.Fatalf("eth_call 缺少 input 字段（geth 1.14 只认 input）: %s", req.Params[0])
	}
	var dataStr, inputStr string
	if err := json.Unmarshal(rawData, &dataStr); err != nil {
		t.Fatalf("decode data %s: %v", rawData, err)
	}
	if err := json.Unmarshal(rawInput, &inputStr); err != nil {
		t.Fatalf("decode input %s: %v", rawInput, err)
	}
	if dataStr != hexutil.Encode(data) {
		t.Fatalf("data = %s, want %s", dataStr, hexutil.Encode(data))
	}
	if inputStr != dataStr {
		t.Fatalf("data 与 input 不一致: data=%s input=%s", dataStr, inputStr)
	}
	if to := callArg["to"]; to == nil {
		t.Fatalf("eth_call 缺少 to 字段: %s", req.Params[0])
	}

	// 关键断言：按 chain33 v1.72 的 rpc/ethrpc/types.CallMsg（只有 json:"data"）反序列化，
	// calldata 必须还在——否则节点会以空 calldata 执行，合约 fallback revert 后返回空结果。
	var chain33CallMsg struct {
		From string         `json:"from"`
		To   string         `json:"to"`
		Data *hexutil.Bytes `json:"data"`
	}
	if err := json.Unmarshal(req.Params[0], &chain33CallMsg); err != nil {
		t.Fatalf("decode call arg as chain33 CallMsg: %v", err)
	}
	if chain33CallMsg.Data == nil || len(*chain33CallMsg.Data) == 0 {
		t.Fatalf("chain33 侧解不出 calldata（data 字段缺失）: %s", req.Params[0])
	}
	if hexutil.Encode(*chain33CallMsg.Data) != hexutil.Encode(data) {
		t.Fatalf("chain33 侧 calldata = %s, want %s", hexutil.Encode(*chain33CallMsg.Data), hexutil.Encode(data))
	}
}
