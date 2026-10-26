package database

import (
	"database/sql"
	"fmt"
	"math/big"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// FunctionSignature 函数签名模型
type FunctionSignature struct {
	ID                uint64    `db:"id"`
	Selector          string    `db:"selector"`
	FunctionName      string    `db:"function_name"`
	FunctionSignature string    `db:"function_signature"`
	StandardType      string    `db:"standard_type"`
	FunctionType      string    `db:"function_type"`
	Description       string    `db:"description"`
	IsStandard        bool      `db:"is_standard"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

// Contract 合约模型
type Contract struct {
	ID                 uint64    `db:"id"`
	ContractAddress    string    `db:"contract_address"`
	ContractName       string    `db:"contract_name"`
	ContractSymbol     string    `db:"contract_symbol"`
	ContractType       string    `db:"contract_type"` // ERC20/ERC721/ERC1155/UNKNOWN等
	Decimals           uint8     `db:"decimals"`
	TotalSupply        *big.Int  `db:"total_supply"`
	DeployTxHash       string    `db:"deploy_tx_hash"`
	DeployBlockNumber  uint64    `db:"deploy_block_number"`
	DeployBlockTime    time.Time `db:"deploy_block_time"`
	DeployerAddress    string    `db:"deployer_address"`
	VerificationStatus int8      `db:"verification_status"`
	VerifiedFunctions  string    `db:"verified_functions"` // JSON格式的函数选择器列表
	CreatedAt          time.Time `db:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"`
}

// Transaction 交易模型
type Transaction struct {
	ID              uint64    `db:"id"`
	TxHash          string    `db:"tx_hash"`
	BlockNumber     uint64    `db:"block_number"`
	BlockHash       string    `db:"block_hash"`
	BlockTime       time.Time `db:"block_time"`
	TxIndex         uint      `db:"tx_index"`
	FromAddress     string    `db:"from_address"`
	ToAddress       string    `db:"to_address"`
	ContractAddress string    `db:"contract_address"`
	FuncSelector    string    `db:"func_selector"`
	FuncName        string    `db:"func_name"`
	Value           *big.Int  `db:"value"`
	GasLimit        uint64    `db:"gas_limit"`
	GasUsed         uint64    `db:"gas_used"`
	GasPrice        *big.Int  `db:"gas_price"`
	TxFee           *big.Int  `db:"tx_fee"`
	Status          int8      `db:"status"`
	TxData          string    `db:"tx_data"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// TokenBalance 代币余额模型
type TokenBalance struct {
	ID                uint64     `db:"id"`
	Address           string     `db:"address"`
	ContractAddress   string     `db:"contract_address"`
	Balance           *big.Int   `db:"balance"`
	BalanceFormatted  *big.Float `db:"balance_formatted"`
	LastTxHash        string     `db:"last_tx_hash"`
	LastTxBlockNumber uint64     `db:"last_tx_block_number"`
	LastUpdatedAt     time.Time  `db:"last_updated_at"`
	CreatedAt         time.Time  `db:"created_at"`
}

// AccountERC20BalanceRow 某用户地址持有的一笔 ERC20（token_balances JOIN contracts，仅 contract_type=ERC20）
type AccountERC20BalanceRow struct {
	ContractAddress string
	ContractName    string
	ContractSymbol  string
	Decimals        uint8
	Balance         *big.Int
	LastTxHash      string
	LastTxBlock     uint64
	LastUpdatedAt   time.Time
}

// Event 事件模型
type Event struct {
	ID              uint64     `db:"id"`
	TxHash          string     `db:"tx_hash"`
	BlockNumber     uint64     `db:"block_number"`
	BlockTime       time.Time  `db:"block_time"`
	LogIndex        uint       `db:"log_index"`
	ContractAddress string     `db:"contract_address"`
	EventName       string     `db:"event_name"`
	EventSignature  string     `db:"event_signature"`
	FromAddress     string     `db:"from_address"`
	ToAddress       string     `db:"to_address"`
	Value           *big.Int   `db:"value"`
	ValueFormatted  *big.Float `db:"value_formatted"`
	OwnerAddress    string     `db:"owner_address"`
	SpenderAddress  string     `db:"spender_address"`
	Amount          *big.Int   `db:"amount"`
	Topic0          string     `db:"topic0"`
	Topic1          string     `db:"topic1"`
	Topic2          string     `db:"topic2"`
	Topic3          string     `db:"topic3"`
	Data            string     `db:"data"`
	CreatedAt       time.Time  `db:"created_at"`
}

// ScanProgress 扫描进度模型
type ScanProgress struct {
	ID               uint64    `db:"id"`
	LastBlockNumber  uint64    `db:"last_block_number"`
	LastBlockHash    string    `db:"last_block_hash"`
	LastBlockTime    time.Time `db:"last_block_time"`
	ProcessedTxCount uint64    `db:"processed_tx_count"`
	UpdatedAt        time.Time `db:"updated_at"`
	CreatedAt        time.Time `db:"created_at"`
}

// SeqMarker 扫块进度标记（append-only，一个 seq 一条，游标 = max(seq)）
//
// 与 ES 侧 db/seqmark 同一套语义：只写不删、del seq 也写（游标不回退），
// 缺号即数据空洞，可用于对账；重复处理同一 seq 时按主键覆盖（幂等）。
type SeqMarker struct {
	Seq    uint64
	Height uint64
	Hash   string
	Type   int
}

// InitScanSeqMarkerTable 初始化扫块进度表
func (db *DB) InitScanSeqMarkerTable() error {
	query := `CREATE TABLE IF NOT EXISTS scan_seq_marker (
		seq BIGINT UNSIGNED NOT NULL COMMENT 'chain33 seq（主键，append-only）',
		height BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '区块高度（del seq 可能为 0）',
		hash VARCHAR(66) NOT NULL DEFAULT '' COMMENT '区块哈希',
		type TINYINT NOT NULL DEFAULT 0 COMMENT '1=add 2=del（chain33 seq type）',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		PRIMARY KEY (seq),
		KEY idx_height (height)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='扫块 seq 进度标记'`

	_, err := db.exec(query)
	return err
}

// SaveSeqMarker 写入一个 seq 的进度（幂等：同一 seq 重复处理时覆盖）
func (db *DB) SaveSeqMarker(m *SeqMarker) error {
	if m == nil {
		return fmt.Errorf("nil seq marker")
	}
	if err := db.InitScanSeqMarkerTable(); err != nil {
		return fmt.Errorf("init scan_seq_marker: %w", err)
	}
	query := `INSERT INTO scan_seq_marker (seq, height, hash, type) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE height = VALUES(height), hash = VALUES(hash), type = VALUES(type)`
	_, err := db.exec(query, m.Seq, m.Height, m.Hash, m.Type)
	return err
}

// MaxSeqMarker 取当前游标（最后一条 marker）；无记录返回 (nil, nil)
func (db *DB) MaxSeqMarker() (*SeqMarker, error) {
	if err := db.InitScanSeqMarkerTable(); err != nil {
		return nil, fmt.Errorf("init scan_seq_marker: %w", err)
	}
	query := `SELECT seq, height, hash, type FROM scan_seq_marker ORDER BY seq DESC LIMIT 1`
	m := &SeqMarker{}
	err := db.queryRow(query).Scan(&m.Seq, &m.Height, &m.Hash, &m.Type)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// DeleteSeqMarkerBefore 滚动删除旧 marker（seq < before）；before<=0 不删
func (db *DB) DeleteSeqMarkerBefore(before uint64) error {
	if before == 0 {
		return nil
	}
	if err := db.InitScanSeqMarkerTable(); err != nil {
		return fmt.Errorf("init scan_seq_marker: %w", err)
	}
	_, err := db.exec(`DELETE FROM scan_seq_marker WHERE seq < ?`, before)
	return err
}

// DB 数据库操作接口
type DB struct {
	conn *sql.DB
	tx   *sql.Tx // 非 nil 时所有读写走该事务（按块处理用）
}

// exec/query/queryRow：有事务走事务，否则直连连接池
func (db *DB) exec(query string, args ...interface{}) (sql.Result, error) {
	if db.tx != nil {
		return db.tx.Exec(query, args...)
	}
	return db.conn.Exec(query, args...)
}

func (db *DB) query(query string, args ...interface{}) (*sql.Rows, error) {
	if db.tx != nil {
		return db.tx.Query(query, args...)
	}
	return db.conn.Query(query, args...)
}

func (db *DB) queryRow(query string, args ...interface{}) *sql.Row {
	if db.tx != nil {
		return db.tx.QueryRow(query, args...)
	}
	return db.conn.QueryRow(query, args...)
}

// BeginBlock 开始"按块处理"的事务；已在事务中则直接复用（返回 false 表示复用）。
func (db *DB) BeginBlock() (bool, error) {
	if db.tx != nil {
		return false, nil
	}
	t, err := db.conn.Begin()
	if err != nil {
		return false, err
	}
	db.tx = t
	return true, nil
}

// CommitBlock 提交按块事务；fresh=false 表示事务是外层开的，不提交。
func (db *DB) CommitBlock(fresh bool) error {
	if !fresh || db.tx == nil {
		return nil
	}
	err := db.tx.Commit()
	db.tx = nil
	return err
}

// RollbackBlock 回滚按块事务（fresh=false 时不动）。
func (db *DB) RollbackBlock(fresh bool) {
	if !fresh || db.tx == nil {
		return
	}
	_ = db.tx.Rollback()
	db.tx = nil
}

// DeleteBlockData 删除某个高度已写入的行（幂等替换：先删后插，用于重放/回滚）。
// 删除顺序受外键约束：events.tx_hash → transactions.tx_hash（ON DELETE RESTRICT），必须先删 events。
func (db *DB) DeleteBlockData(height uint64) (events int64, txs int64, err error) {
	res, err := db.exec(`DELETE FROM events WHERE block_number = ?`, height)
	if err != nil {
		return 0, 0, fmt.Errorf("delete events height=%d: %w", height, err)
	}
	events, _ = res.RowsAffected()
	res, err = db.exec(`DELETE FROM transactions WHERE block_number = ?`, height)
	if err != nil {
		return events, 0, fmt.Errorf("delete transactions height=%d: %w", height, err)
	}
	txs, _ = res.RowsAffected()
	return events, txs, nil
}

// GetConn 获取数据库连接（用于复杂查询）
func (db *DB) GetConn() *sql.DB {
	return db.conn
}

// NewDB 创建数据库连接
func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &DB{conn: db}, nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.conn.Close()
}

// InitScanProgressTable 初始化扫描进度表
func (db *DB) InitScanProgressTable() error {
	query := `CREATE TABLE IF NOT EXISTS scan_progress (
		id BIGINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '主键ID(固定为1，确保只有一条记录)',
		last_block_number BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '最后处理的区块号',
		last_block_hash VARCHAR(66) DEFAULT NULL COMMENT '最后处理的区块哈希',
		last_block_time DATETIME DEFAULT NULL COMMENT '最后处理的区块时间',
		processed_tx_count BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '已处理的交易总数',
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='扫描进度表'`

	_, err := db.exec(query)
	if err != nil {
		return err
	}

	// 确保有一条初始记录
	insertQuery := `INSERT IGNORE INTO scan_progress (id, last_block_number, processed_tx_count) VALUES (1, 0, 0)`
	_, err = db.exec(insertQuery)
	return err
}

// GetScanProgress 获取扫描进度
func (db *DB) GetScanProgress() (*ScanProgress, error) {
	// 确保表存在
	if err := db.InitScanProgressTable(); err != nil {
		return nil, fmt.Errorf("failed to init scan progress table: %w", err)
	}

	query := `SELECT * FROM scan_progress ORDER BY id DESC LIMIT 1`

	progress := &ScanProgress{}
	err := db.queryRow(query).Scan(
		&progress.ID,
		&progress.LastBlockNumber,
		&progress.LastBlockHash,
		&progress.LastBlockTime,
		&progress.ProcessedTxCount,
		&progress.UpdatedAt,
		&progress.CreatedAt,
	)

	if err == sql.ErrNoRows {
		// 如果没有记录，返回nil，表示需要从配置的起始高度开始
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return progress, nil
}

// UpdateScanProgress 更新扫描进度
func (db *DB) UpdateScanProgress(blockNumber uint64, blockHash string, blockTime time.Time, processedTxCount uint64) error {
	// 确保表存在
	if err := db.InitScanProgressTable(); err != nil {
		return fmt.Errorf("failed to init scan progress table: %w", err)
	}

	// 使用 INSERT ... ON DUPLICATE KEY UPDATE 来确保只有一条记录
	query := `INSERT INTO scan_progress (
		id, last_block_number, last_block_hash, last_block_time, processed_tx_count
	) VALUES (1, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		last_block_number = VALUES(last_block_number),
		last_block_hash = VALUES(last_block_hash),
		last_block_time = VALUES(last_block_time),
		processed_tx_count = VALUES(processed_tx_count),
		updated_at = CURRENT_TIMESTAMP`

	_, err := db.exec(query, blockNumber, blockHash, blockTime, processedTxCount)
	return err
}

// GetFunctionSignature 根据选择器获取函数签名
func (db *DB) GetFunctionSignature(selector string) (*FunctionSignature, error) {
	query := `SELECT * FROM function_signatures WHERE selector = ?`

	sig := &FunctionSignature{}
	err := db.queryRow(query, selector).Scan(
		&sig.ID,
		&sig.Selector,
		&sig.FunctionName,
		&sig.FunctionSignature,
		&sig.StandardType,
		&sig.FunctionType,
		&sig.Description,
		&sig.IsStandard,
		&sig.CreatedAt,
		&sig.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// GetFunctionSignaturesByStandard 根据标准类型获取函数签名列表
func (db *DB) GetFunctionSignaturesByStandard(standardType string) ([]FunctionSignature, error) {
	query := `SELECT * FROM function_signatures WHERE standard_type = ? ORDER BY function_name`

	rows, err := db.query(query, standardType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var signatures []FunctionSignature
	for rows.Next() {
		var sig FunctionSignature
		err := rows.Scan(
			&sig.ID,
			&sig.Selector,
			&sig.FunctionName,
			&sig.FunctionSignature,
			&sig.StandardType,
			&sig.FunctionType,
			&sig.Description,
			&sig.IsStandard,
			&sig.CreatedAt,
			&sig.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		signatures = append(signatures, sig)
	}

	return signatures, nil
}

// SaveContract 保存合约信息
func (db *DB) SaveContract(contract *Contract) error {
	query := `INSERT INTO contracts (
		contract_address, contract_name, contract_symbol, contract_type, decimals, 
		total_supply, deploy_tx_hash, deploy_block_number, deploy_block_time,
		deployer_address, verification_status, verified_functions
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		contract_name = VALUES(contract_name),
		contract_symbol = VALUES(contract_symbol),
		contract_type = VALUES(contract_type),
		decimals = VALUES(decimals),
		total_supply = VALUES(total_supply),
		deploy_tx_hash = IF(VALUES(deploy_tx_hash) != "", VALUES(deploy_tx_hash), deploy_tx_hash),
		deploy_block_number = IF(VALUES(deploy_block_number) > 0, VALUES(deploy_block_number), deploy_block_number),
		deploy_block_time = IF(VALUES(deploy_block_time) IS NOT NULL, VALUES(deploy_block_time), deploy_block_time),
		deployer_address = IF(VALUES(deployer_address) != "", VALUES(deployer_address), deployer_address),
		verification_status = VALUES(verification_status),
		verified_functions = VALUES(verified_functions),
		updated_at = CURRENT_TIMESTAMP`

	// 处理 total_supply 字段：如果为 nil，传递 nil（SQL NULL），否则传递字符串
	var totalSupplyStr interface{}
	if contract.TotalSupply != nil {
		totalSupplyStr = clampAmountForDB(contract.TotalSupply).String()
	} else {
		totalSupplyStr = nil
	}

	_, err := db.exec(query,
		contract.ContractAddress,
		contract.ContractName,
		contract.ContractSymbol,
		contract.ContractType,
		contract.Decimals,
		totalSupplyStr,
		contract.DeployTxHash,
		contract.DeployBlockNumber,
		contract.DeployBlockTime,
		contract.DeployerAddress,
		contract.VerificationStatus,
		contract.VerifiedFunctions,
	)
	return err
}

// SaveTransaction 保存交易信息
func (db *DB) SaveTransaction(tx *Transaction) error {
	query := `INSERT INTO transactions (
		tx_hash, block_number, block_hash, block_time, tx_index,
		from_address, to_address, contract_address, func_selector, func_name,
		value, gas_limit, gas_used, gas_price, tx_fee, status, tx_data
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		status = VALUES(status),
		gas_used = VALUES(gas_used),
		func_selector = IF(VALUES(func_selector) != '', VALUES(func_selector), func_selector),
		func_name = IF(VALUES(func_name) != '', VALUES(func_name), func_name),
		updated_at = CURRENT_TIMESTAMP`

	// 处理 DECIMAL 字段：如果为 nil，传递 nil（SQL NULL），否则传递字符串
	var valueStr interface{}
	if tx.Value != nil {
		valueStr = clampAmountForDB(tx.Value).String()
	} else {
		valueStr = nil
	}

	var gasPriceStr interface{}
	if tx.GasPrice != nil {
		gasPriceStr = tx.GasPrice.String()
	} else {
		gasPriceStr = nil
	}

	var txFeeStr interface{}
	if tx.TxFee != nil {
		txFeeStr = tx.TxFee.String()
	} else {
		txFeeStr = nil
	}

	_, err := db.exec(query,
		tx.TxHash,
		tx.BlockNumber,
		tx.BlockHash,
		tx.BlockTime,
		tx.TxIndex,
		tx.FromAddress,
		tx.ToAddress,
		tx.ContractAddress,
		tx.FuncSelector,
		tx.FuncName,
		valueStr,
		tx.GasLimit,
		tx.GasUsed,
		gasPriceStr,
		txFeeStr,
		tx.Status,
		tx.TxData,
	)
	return err
}

// UpdateTokenBalance 更新代币余额
func (db *DB) UpdateTokenBalance(address, contractAddress string, balance *big.Int, txHash string, blockNumber uint64) error {
	query := `INSERT INTO token_balances (
		address, contract_address, balance, last_tx_hash, last_tx_block_number
	) VALUES (?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		balance = VALUES(balance),
		last_tx_hash = VALUES(last_tx_hash),
		last_tx_block_number = VALUES(last_tx_block_number),
		last_updated_at = CURRENT_TIMESTAMP`

	_, err := db.exec(query,
		address,
		contractAddress,
		clampAmountForDB(balance).String(),
		txHash,
		blockNumber,
	)
	return err
}

// TokenBalanceRowKey (address, contract) 用于后台刷新任务选批
type TokenBalanceRowKey struct {
	Address         string
	ContractAddress string
}

// ListTokenBalanceRowsForRefresh 选取待刷新行：仅 ERC20，按 last_updated_at 最旧优先
func (db *DB) ListTokenBalanceRowsForRefresh(limit int, minAge time.Duration) ([]TokenBalanceRowKey, error) {
	var query string
	var args []interface{}
	if minAge <= 0 {
		query = `
			SELECT tb.address, tb.contract_address
			FROM token_balances tb
			INNER JOIN contracts c ON tb.contract_address = c.contract_address
			WHERE c.contract_type = 'ERC20'
			ORDER BY tb.last_updated_at ASC
			LIMIT ?`
		args = []interface{}{limit}
	} else {
		cutoff := time.Now().Add(-minAge)
		query = `
			SELECT tb.address, tb.contract_address
			FROM token_balances tb
			INNER JOIN contracts c ON tb.contract_address = c.contract_address
			WHERE c.contract_type = 'ERC20'
			  AND tb.last_updated_at < ?
			ORDER BY tb.last_updated_at ASC
			LIMIT ?`
		args = []interface{}{cutoff, limit}
	}
	rows, err := db.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TokenBalanceRowKey
	for rows.Next() {
		var k TokenBalanceRowKey
		if err := rows.Scan(&k.Address, &k.ContractAddress); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpdateTokenBalanceFromChain 仅更新链上查询得到的余额与 last_updated_at，不修改 last_tx_*
func (db *DB) UpdateTokenBalanceFromChain(address, contractAddress string, balance *big.Int) error {
	if balance == nil {
		balance = big.NewInt(0)
	}
	q := `UPDATE token_balances SET balance = ?, last_updated_at = CURRENT_TIMESTAMP
		WHERE address = ? AND contract_address = ?`
	_, err := db.exec(q, clampAmountForDB(balance).String(), address, contractAddress)
	return err
}

// UpsertTokenBalanceMetadata 插入新行 balance=0，或已存在时仅更新最后交易元数据（不覆盖已有 balance）
func (db *DB) UpsertTokenBalanceMetadata(address, contractAddress, txHash string, blockNumber uint64) error {
	q := `INSERT INTO token_balances (
			address, contract_address, balance, last_tx_hash, last_tx_block_number
		) VALUES (?, ?, '0', ?, ?)
		ON DUPLICATE KEY UPDATE
			last_tx_hash = VALUES(last_tx_hash),
			last_tx_block_number = VALUES(last_tx_block_number),
			last_updated_at = CURRENT_TIMESTAMP`
	_, err := db.exec(q, address, contractAddress, txHash, blockNumber)
	return err
}

// SaveEvent 保存事件信息
func (db *DB) SaveEvent(event *Event) error {
	query := `INSERT INTO events (
		tx_hash, block_number, block_time, log_index, contract_address,
		event_name, event_signature, from_address, to_address, value,
		owner_address, spender_address, amount, topic0, topic1, topic2, topic3, data
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		value = VALUES(value),
		amount = VALUES(amount)`

	// 处理 value 字段：如果为 nil，传递 nil（SQL NULL），否则传递字符串
	var valueStr interface{}
	if event.Value != nil {
		valueStr = clampAmountForDB(event.Value).String()
	} else {
		valueStr = nil
	}

	// 处理 amount 字段：如果为 nil，传递 nil（SQL NULL），否则传递字符串
	var amountStr interface{}
	if event.Amount != nil {
		amountStr = clampAmountForDB(event.Amount).String()
	} else {
		amountStr = nil
	}

	_, err := db.exec(query,
		event.TxHash,
		event.BlockNumber,
		event.BlockTime,
		event.LogIndex,
		event.ContractAddress,
		event.EventName,
		event.EventSignature,
		event.FromAddress,
		event.ToAddress,
		valueStr,
		event.OwnerAddress,
		event.SpenderAddress,
		amountStr,
		event.Topic0,
		event.Topic1,
		event.Topic2,
		event.Topic3,
		event.Data,
	)
	return err
}

// GetContractByAddress 根据地址获取合约信息（大小写不敏感）
func (db *DB) GetContractByAddress(address string) (*Contract, error) {
	query := `SELECT * FROM contracts WHERE contract_address = ?`

	contract := &Contract{}
	var totalSupplyStr sql.NullString
	err := db.queryRow(query, address).Scan(
		&contract.ID,
		&contract.ContractAddress,
		&contract.ContractName,
		&contract.ContractSymbol,
		&contract.ContractType,
		&contract.Decimals,
		&totalSupplyStr,
		&contract.DeployTxHash,
		&contract.DeployBlockNumber,
		&contract.DeployBlockTime,
		&contract.DeployerAddress,
		&contract.VerificationStatus,
		&contract.VerifiedFunctions,
		&contract.CreatedAt,
		&contract.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// 将字符串转换为 big.Int
	if totalSupplyStr.Valid && totalSupplyStr.String != "" {
		totalSupply, ok := new(big.Int).SetString(totalSupplyStr.String, 10)
		if !ok {
			return nil, fmt.Errorf("failed to parse total_supply: %s", totalSupplyStr.String)
		}
		contract.TotalSupply = totalSupply
	}

	return contract, nil
}

// GetTokenBalancesByAddress 获取地址的所有代币余额
func (db *DB) GetTokenBalancesByAddress(address string) ([]TokenBalance, error) {
	query := `SELECT * FROM token_balances WHERE address = ? ORDER BY balance DESC`

	rows, err := db.query(query, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []TokenBalance
	for rows.Next() {
		var balance TokenBalance
		var balanceStr sql.NullString
		var balanceFormattedStr sql.NullString
		err := rows.Scan(
			&balance.ID,
			&balance.Address,
			&balance.ContractAddress,
			&balanceStr,
			&balanceFormattedStr,
			&balance.LastTxHash,
			&balance.LastTxBlockNumber,
			&balance.LastUpdatedAt,
			&balance.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// 将字符串转换为 big.Int
		if balanceStr.Valid && balanceStr.String != "" {
			balanceValue, ok := new(big.Int).SetString(balanceStr.String, 10)
			if ok {
				balance.Balance = balanceValue
			}
		}

		// 将字符串转换为 big.Float
		if balanceFormattedStr.Valid && balanceFormattedStr.String != "" {
			balanceFormattedValue, _, err := new(big.Float).Parse(balanceFormattedStr.String, 10)
			if err == nil {
				balance.BalanceFormatted = balanceFormattedValue
			}
		}

		balances = append(balances, balance)
	}

	return balances, nil
}

// ListERC20BalancesByHolderAddress 分页查询某地址持有的 ERC20 代币及余额（仅 contracts 中标记为 ERC20 的合约）
func (db *DB) ListERC20BalancesByHolderAddress(holder string, minBalance string, limit, offset int) ([]AccountERC20BalanceRow, int, error) {
	baseWhere := `FROM token_balances tb
		INNER JOIN contracts c ON tb.contract_address = c.contract_address
		WHERE tb.address = ? AND c.contract_type = 'ERC20'`
	args := []interface{}{holder}
	if minBalance != "" {
		baseWhere += ` AND tb.balance >= ?`
		args = append(args, minBalance)
	}

	countQuery := `SELECT COUNT(*) ` + baseWhere
	var total int
	if err := db.queryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `SELECT tb.contract_address,
		COALESCE(c.contract_name, ''),
		COALESCE(c.contract_symbol, ''),
		c.decimals,
		tb.balance,
		tb.last_tx_hash,
		tb.last_tx_block_number,
		tb.last_updated_at ` + baseWhere + `
		ORDER BY tb.balance DESC LIMIT ? OFFSET ?`
	listArgs := append(append([]interface{}(nil), args...), limit, offset)

	rows, err := db.query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []AccountERC20BalanceRow
	for rows.Next() {
		var row AccountERC20BalanceRow
		var balStr sql.NullString
		if err := rows.Scan(
			&row.ContractAddress,
			&row.ContractName,
			&row.ContractSymbol,
			&row.Decimals,
			&balStr,
			&row.LastTxHash,
			&row.LastTxBlock,
			&row.LastUpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if balStr.Valid && balStr.String != "" {
			v, ok := new(big.Int).SetString(balStr.String, 10)
			if ok {
				row.Balance = v
			}
		}
		if row.Balance == nil {
			row.Balance = big.NewInt(0)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetEventsByContract 获取合约的所有事件
func (db *DB) GetEventsByContract(contractAddress string, limit int) ([]Event, error) {
	query := `SELECT * FROM events 
		WHERE contract_address = ? 
		ORDER BY block_number DESC, log_index DESC 
		LIMIT ?`

	rows, err := db.query(query, contractAddress, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var valueStr sql.NullString
		var valueFormattedStr sql.NullString
		var amountStr sql.NullString
		err := rows.Scan(
			&event.ID,
			&event.TxHash,
			&event.BlockNumber,
			&event.BlockTime,
			&event.LogIndex,
			&event.ContractAddress,
			&event.EventName,
			&event.EventSignature,
			&event.FromAddress,
			&event.ToAddress,
			&valueStr,
			&valueFormattedStr,
			&event.OwnerAddress,
			&event.SpenderAddress,
			&amountStr,
			&event.Topic0,
			&event.Topic1,
			&event.Topic2,
			&event.Topic3,
			&event.Data,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// 将字符串转换为 big.Int (value)
		if valueStr.Valid && valueStr.String != "" {
			value, ok := new(big.Int).SetString(valueStr.String, 10)
			if ok {
				event.Value = value
			}
		}

		// 将字符串转换为 big.Float (value_formatted)
		if valueFormattedStr.Valid && valueFormattedStr.String != "" {
			valueFormatted, _, err := new(big.Float).Parse(valueFormattedStr.String, 10)
			if err == nil {
				event.ValueFormatted = valueFormatted
			}
		}

		// 将字符串转换为 big.Int (amount)
		if amountStr.Valid && amountStr.String != "" {
			amount, ok := new(big.Int).SetString(amountStr.String, 10)
			if ok {
				event.Amount = amount
			}
		}

		events = append(events, event)
	}

	return events, nil
}

// TokenAllowance 授权额度模型
type TokenAllowance struct {
	Owner           string    `db:"owner"`
	Spender         string    `db:"spender"`
	ContractAddress string    `db:"contract_address"`
	Amount          *big.Int  `db:"amount"`
	LastTxHash      string    `db:"last_tx_hash"`
	LastBlockNumber uint64    `db:"last_block_number"`
	LastUpdatedAt   time.Time `db:"last_updated_at"`
	CreatedAt       time.Time `db:"created_at"`
}

// AllowanceRow 授权查询结果行（JOIN contracts）
type AllowanceRow struct {
	Owner           string
	Spender         string
	ContractAddress string
	ContractName    string
	ContractSymbol  string
	Decimals        uint8
	Amount          *big.Int
	LastTxHash      string
	LastBlockNumber uint64
	LastUpdatedAt   time.Time
}

// InitTokenAllowancesTable 初始化授权额度表
func (db *DB) InitTokenAllowancesTable() error {
	query := `CREATE TABLE IF NOT EXISTS token_allowances (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
		owner VARCHAR(42) NOT NULL COMMENT '授权方地址',
		spender VARCHAR(42) NOT NULL COMMENT '被授权方地址',
		contract_address VARCHAR(42) NOT NULL COMMENT 'ERC20合约地址',
		amount DECIMAL(65,0) NOT NULL DEFAULT '0' COMMENT '授权额度（原始值）',
		last_tx_hash VARCHAR(66) DEFAULT NULL COMMENT '最近一笔Approval事件tx hash',
		last_block_number BIGINT UNSIGNED DEFAULT NULL COMMENT '最近一笔Approval事件区块号',
		last_updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
		PRIMARY KEY (id),
		UNIQUE KEY uk_owner_spender_contract (owner, spender, contract_address),
		KEY idx_owner_amount (owner, amount)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='ERC20授权额度表'`

	_, err := db.exec(query)
	return err
}

// UpsertAllowance 插入或更新授权额度
func (db *DB) UpsertAllowance(owner, spender, contractAddress string, amount *big.Int, txHash string, blockNumber uint64) error {
	if err := db.InitTokenAllowancesTable(); err != nil {
		return fmt.Errorf("failed to init token_allowances table: %w", err)
	}

	query := `INSERT INTO token_allowances (
		owner, spender, contract_address, amount, last_tx_hash, last_block_number
	) VALUES (?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		amount = VALUES(amount),
		last_tx_hash = VALUES(last_tx_hash),
		last_block_number = VALUES(last_block_number),
		last_updated_at = CURRENT_TIMESTAMP`

	_, err := db.exec(query,
		owner,
		spender,
		contractAddress,
		clampAmountForDB(amount).String(),
		txHash,
		blockNumber,
	)
	return err
}

// DeleteAllowance 删除授权记录（amount=0 时调用）
func (db *DB) DeleteAllowance(owner, spender, contractAddress string) error {
	query := `DELETE FROM token_allowances WHERE owner = ? AND spender = ? AND contract_address = ?`
	_, err := db.exec(query, owner, spender, contractAddress)
	return err
}

// ListAllowancesByOwner 按 owner 地址查询所有授权记录
func (db *DB) ListAllowancesByOwner(owner string, minAmount string, contractFilter string, limit, offset int) ([]AllowanceRow, int, error) {
	if err := db.InitTokenAllowancesTable(); err != nil {
		return nil, 0, fmt.Errorf("failed to init token_allowances table: %w", err)
	}

	baseWhere := `FROM token_allowances ta
		LEFT JOIN contracts c ON ta.contract_address = c.contract_address
		WHERE ta.owner = ?`
	args := []interface{}{owner}

	if minAmount != "" {
		baseWhere += ` AND ta.amount >= ?`
		args = append(args, minAmount)
	}
	if contractFilter != "" {
		baseWhere += ` AND ta.contract_address = ?`
		args = append(args, contractFilter)
	}

	// 统计总数
	countQuery := `SELECT COUNT(*) ` + baseWhere
	var total int
	if err := db.queryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 查询列表
	listQuery := `SELECT ta.owner, ta.spender, ta.contract_address,
		COALESCE(c.contract_name, ''),
		COALESCE(c.contract_symbol, ''),
		c.decimals,
		ta.amount,
		ta.last_tx_hash,
		COALESCE(ta.last_block_number, 0),
		ta.last_updated_at ` + baseWhere + `
		ORDER BY ta.amount DESC LIMIT ? OFFSET ?`
	listArgs := append(append([]interface{}(nil), args...), limit, offset)

	rows, err := db.query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []AllowanceRow
	for rows.Next() {
		var row AllowanceRow
		var amountStr sql.NullString
		if err := rows.Scan(
			&row.Owner,
			&row.Spender,
			&row.ContractAddress,
			&row.ContractName,
			&row.ContractSymbol,
			&row.Decimals,
			&amountStr,
			&row.LastTxHash,
			&row.LastBlockNumber,
			&row.LastUpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if amountStr.Valid && amountStr.String != "" {
			v, ok := new(big.Int).SetString(amountStr.String, 10)
			if ok {
				row.Amount = v
			}
		}
		if row.Amount == nil {
			row.Amount = big.NewInt(0)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// EnsureContract 确保合约地址在 contracts 表中有对应行。
// 已存在则不做任何修改，不存在则 INSERT 占位行（contract_type=UNKNOWN）。
// 保留 fk_tx_contract 外键约束，保证 transactions 写入前合约已存在。
func (db *DB) EnsureContract(contractAddress string) error {
	query := `INSERT IGNORE INTO contracts
		(contract_address, contract_name, contract_symbol, contract_type,
		 decimals, verification_status, verified_functions)
		VALUES (?, "Unknown", "UNKNOWN", "UNKNOWN", 0, 0, "[]")`
	_, err := db.exec(query, contractAddress)
	return err
}

// TransactionListItem 交易列表查询返回的轻量结构（不含 tx_data 大字段）。
type TransactionListItem struct {
	TxHash          string
	BlockNumber     uint64
	BlockTime       time.Time
	FromAddress     string
	ToAddress       string
	ContractAddress string
	FuncSelector    string
	FuncName        string
	Value           *big.Int
	GasLimit        uint64
	GasUsed         uint64
	GasPrice        *big.Int
	TxFee           *big.Int
	Status          int8
}

// ListTransactions 查询交易列表，按 block_number DESC 排序。
// contract 和 from 均为空字符串时视为不限制。
func (db *DB) ListTransactions(contract, from string, startBlock, endBlock uint64, page, pageSize int) ([]TransactionListItem, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}

	if contract != "" {
		where += " AND to_address = ?"
		args = append(args, contract)
	}
	if from != "" {
		where += " AND from_address = ?"
		args = append(args, from)
	}
	if startBlock > 0 {
		where += " AND block_number >= ?"
		args = append(args, startBlock)
	}
	if endBlock > 0 {
		where += " AND block_number <= ?"
		args = append(args, endBlock)
	}

	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	var total int
	countQuery := "SELECT COUNT(*) FROM transactions " + where
	if err := db.queryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	listQuery := `SELECT tx_hash, block_number, block_time, from_address, to_address,
		contract_address, func_selector, func_name, value,
		gas_limit, gas_used, gas_price, tx_fee, status
		FROM transactions ` + where + ` ORDER BY block_number DESC LIMIT ? OFFSET ?`
	listArgs := append(args, pageSize, offset)

	rows, err := db.query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []TransactionListItem
	for rows.Next() {
		var item TransactionListItem
		var valueStr, gasPriceStr, txFeeStr sql.NullString
		err := rows.Scan(
			&item.TxHash,
			&item.BlockNumber,
			&item.BlockTime,
			&item.FromAddress,
			&item.ToAddress,
			&item.ContractAddress,
			&item.FuncSelector,
			&item.FuncName,
			&valueStr,
			&item.GasLimit,
			&item.GasUsed,
			&gasPriceStr,
			&txFeeStr,
			&item.Status,
		)
		if err != nil {
			return nil, 0, err
		}
		if valueStr.Valid && valueStr.String != "" {
			item.Value, _ = new(big.Int).SetString(valueStr.String, 10)
		}
		if gasPriceStr.Valid && gasPriceStr.String != "" {
			item.GasPrice, _ = new(big.Int).SetString(gasPriceStr.String, 10)
		}
		if txFeeStr.Valid && txFeeStr.String != "" {
			item.TxFee, _ = new(big.Int).SetString(txFeeStr.String, 10)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
