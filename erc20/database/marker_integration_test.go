//go:build integration

// MySQL 集成测试：需要真实 MySQL（默认本机 3307 的临时实例）。
//
//	docker run -d --name exdb-test-mysql -e MYSQL_ROOT_PASSWORD=testpw \
//	  -e MYSQL_DATABASE=token_scanner -p 127.0.0.1:3307:3306 mysql:5.7
//	TEST_DSN='root:testpw@tcp(127.0.0.1:3307)/token_scanner?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./erc20/database/ -run Marker -v
package database

import (
	"os"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN 未设置，跳过 MySQL 集成测试")
	}
	db, err := NewDB(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestScanSeqMarkerRoundTrip(t *testing.T) {
	db := testDB(t)
	if _, err := db.conn.Exec("DROP TABLE IF EXISTS scan_seq_marker"); err != nil {
		t.Fatal(err)
	}
	if err := db.InitScanSeqMarkerTable(); err != nil {
		t.Fatalf("init: %v", err)
	}

	// 空表
	if m, err := db.MaxSeqMarker(); err != nil || m != nil {
		t.Fatalf("空表应返回 nil, got %+v err=%v", m, err)
	}

	for i := uint64(10); i <= 12; i++ {
		if err := db.SaveSeqMarker(&SeqMarker{Seq: i, Height: i + 1000, Hash: "0xh", Type: 1}); err != nil {
			t.Fatalf("save seq %d: %v", i, err)
		}
	}
	m, err := db.MaxSeqMarker()
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.Seq != 12 || m.Height != 1012 {
		t.Fatalf("游标应为 seq=12, got %+v", m)
	}

	// 幂等：同一 seq 重写不新增行、内容更新
	if err := db.SaveSeqMarker(&SeqMarker{Seq: 12, Height: 1012, Hash: "0xnew", Type: 2}); err != nil {
		t.Fatalf("resave: %v", err)
	}
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM scan_seq_marker").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("重写同一 seq 不应新增行, rows=%d", n)
	}
	m, _ = db.MaxSeqMarker()
	if m.Hash != "0xnew" || m.Type != 2 {
		t.Fatalf("重写未生效: %+v", m)
	}

	// 滚动删除
	if err := db.DeleteSeqMarkerBefore(11); err != nil {
		t.Fatal(err)
	}
	m, _ = db.MaxSeqMarker()
	if m == nil || m.Seq != 12 {
		t.Fatalf("删除后游标仍应为 12, got %+v", m)
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM scan_seq_marker").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("删除 seq<11 后应剩 2 条, rows=%d", n)
	}
}

// 按块替换：先删 events 再删 transactions；事务回滚后数据仍在
func TestDeleteBlockData(t *testing.T) {
	db := testDB(t)
	if _, err := db.conn.Exec("DROP TABLE IF EXISTS events"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.conn.Exec("DROP TABLE IF EXISTS transactions"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.conn.Exec("DROP TABLE IF EXISTS contracts"); err != nil {
		t.Fatal(err)
	}
	// 最小表结构（带外键，验证删除顺序）
	stmts := []string{
		`CREATE TABLE contracts (contract_address VARCHAR(42) NOT NULL PRIMARY KEY) ENGINE=InnoDB`,
		`CREATE TABLE transactions (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			tx_hash VARCHAR(66) NOT NULL, block_number BIGINT UNSIGNED NOT NULL,
			contract_address VARCHAR(42) DEFAULT NULL,
			UNIQUE KEY uk_tx_hash (tx_hash), KEY idx_block_number (block_number),
			CONSTRAINT fk_tx_contract FOREIGN KEY (contract_address) REFERENCES contracts(contract_address)
				ON DELETE RESTRICT ON UPDATE CASCADE) ENGINE=InnoDB`,
		`CREATE TABLE events (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			tx_hash VARCHAR(66) NOT NULL, block_number BIGINT UNSIGNED NOT NULL, log_index INT UNSIGNED NOT NULL,
			contract_address VARCHAR(42) NOT NULL, UNIQUE KEY uk_tx_log_index (tx_hash, log_index),
			KEY idx_block_number (block_number),
			CONSTRAINT fk_event_tx FOREIGN KEY (tx_hash) REFERENCES transactions(tx_hash)
				ON DELETE RESTRICT ON UPDATE CASCADE,
			CONSTRAINT fk_event_contract FOREIGN KEY (contract_address) REFERENCES contracts(contract_address)
				ON DELETE RESTRICT ON UPDATE CASCADE) ENGINE=InnoDB`,
	}
	for _, q := range stmts {
		if _, err := db.conn.Exec(q); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	seed := func(h uint64) {
		if _, err := db.conn.Exec(`INSERT IGNORE INTO contracts (contract_address) VALUES ('0xc')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.conn.Exec(`INSERT INTO transactions (tx_hash, block_number, contract_address) VALUES (?, ?, '0xc')`, "0xt"+string(rune('0'+h%10)), h); err != nil {
			t.Fatal(err)
		}
		if _, err := db.conn.Exec(`INSERT INTO events (tx_hash, block_number, log_index, contract_address) VALUES (?, ?, 0, '0xc')`, "0xt"+string(rune('0'+h%10)), h); err != nil {
			t.Fatal(err)
		}
	}
	seed(1000)
	seed(1001)

	// 正常删除（事务内）
	fresh, err := db.BeginBlock()
	if err != nil || !fresh {
		t.Fatalf("begin: %v fresh=%v", err, fresh)
	}
	nEvents, nTxs, err := db.DeleteBlockData(1000)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if nEvents != 1 || nTxs != 1 {
		t.Fatalf("应删除 1 条 event + 1 条 tx, got events=%d txs=%d", nEvents, nTxs)
	}
	if err := db.CommitBlock(fresh); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var left int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM transactions WHERE block_number = 1000`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("1000 高度应已清空, left=%d", left)
	}

	// 回滚：删除不生效
	fresh, err = db.BeginBlock()
	if err != nil || !fresh {
		t.Fatalf("begin2: %v", err)
	}
	if _, _, err := db.DeleteBlockData(1001); err != nil {
		t.Fatalf("delete2: %v", err)
	}
	db.RollbackBlock(fresh)
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM transactions WHERE block_number = 1001`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("回滚后 1001 的数据应仍在, left=%d", left)
	}
}
