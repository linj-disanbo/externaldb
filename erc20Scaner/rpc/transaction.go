package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// handleTransactionsRouter 路由分发函数，处理 /evmapi/transactions 路径
func handleTransactionsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 移除前缀 /evmapi/transactions
	path := strings.TrimPrefix(r.URL.Path, "/evmapi/transactions")

	// 移除开头的 /
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "Transaction hash is required")
		return
	}

	parts := strings.Split(path, "/")

	// /evmapi/transactions/{tx_hash}/analysis
	if len(parts) == 2 && parts[1] == "analysis" {
		handleTransactionAnalysis(w, r, parts[0])
		return
	}

	// /evmapi/transactions/{tx_hash} - 如果需要查询交易详情，可以在这里实现
	if len(parts) == 1 {
		// 目前暂不实现，返回提示
		writeError(w, http.StatusNotImplemented, "Transaction detail query is not implemented yet")
		return
	}

	writeError(w, http.StatusNotFound, "Invalid path")
}

// handleTransactionAnalysis 解析并获取单笔 EVM 交易的详细动作
// GET /evmapi/transactions/{tx_hash}/analysis
func handleTransactionAnalysis(w http.ResponseWriter, r *http.Request, txHash string) {
	// 验证交易哈希格式
	if !strings.HasPrefix(strings.ToLower(txHash), "0x") || len(txHash) != 66 {
		writeError(w, http.StatusBadRequest, "Invalid tx_hash format")
		return
	}

	// 从Chain33节点获取交易详情
	detail, err := getTxDetailFromChain33(globalChainGRPC, txHash)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Transaction not found: %v", err))
		return
	}

	// 解析EVM交易
	parsed := parseEvmTx(detail, getAbiFromES, globalChainSymbol)

	writeJSON(w, http.StatusOK, APIResponse{
		Code:    0,
		Message: "Success",
		Data:    parsed,
	})
}

// handleListTransactions 通用交易列表查询
// GET /evmapi/transactions?contract=0x..&from=0x..&start_block=N&end_block=N&page=1&page_size=20
func handleListTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	q := r.URL.Query()
	contract := normalizeAddress(q.Get("contract"))
	from := normalizeAddress(q.Get("from"))

	if contract == "" && from == "" {
		writeError(w, http.StatusBadRequest, "At least one of 'contract' or 'from' is required")
		return
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var startBlock, endBlock uint64
	if v := q.Get("start_block"); v != "" {
		startBlock, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := q.Get("end_block"); v != "" {
		endBlock, _ = strconv.ParseUint(v, 10, 64)
	}

	items, total, err := db.ListTransactions(contract, from, startBlock, endBlock, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	out := make([]GenericTransaction, 0, len(items))
	for _, item := range items {
		t := GenericTransaction{
			TxHash:          item.TxHash,
			BlockNumber:     item.BlockNumber,
			BlockTime:       item.BlockTime.Format("2006-01-02T15:04:05Z"),
			FromAddress:     item.FromAddress,
			ToAddress:       item.ToAddress,
			ContractAddress: item.ContractAddress,
			FuncSelector:    item.FuncSelector,
			FuncName:        item.FuncName,
			GasLimit:        item.GasLimit,
			GasUsed:         item.GasUsed,
			Status:          item.Status,
		}
		if item.Value != nil {
			t.Value = item.Value.String()
		}
		if item.GasPrice != nil {
			t.GasPrice = item.GasPrice.String()
		}
		if item.TxFee != nil {
			t.TxFee = item.TxFee.String()
		}
		out = append(out, t)
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Code:    0,
		Message: "Success",
		Data: map[string]interface{}{
			"transactions": out,
			"page":         page,
			"page_size":    pageSize,
			"total":        total,
		},
	})
}
