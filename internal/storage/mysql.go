package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/Luna-xk/chain-event-listener/internal/model"
)

// MySQL 是基于 database/sql 的持久化实现。
//
// 一致性要点:
//   - 唯一索引 (tx_hash, log_index) + INSERT IGNORE 保证写入幂等,重放不重复。
//   - SaveEvents 在单事务内完成"写事件 + 推进进度",要么全成功要么全回滚。
//   - RevertFrom 直接按 block_number 删除,配合 listener 的重组检测实现回滚。
type MySQL struct {
	db    *sql.DB
	chain string // 用于 listener_state 主键,区分多链进度
}

// NewMySQL 连接 MySQL、校验连通性并建表。
func NewMySQL(ctx context.Context, dsn, chain string) (*MySQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(time.Hour)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	s := &MySQL{db: db, chain: chain}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *MySQL) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS chain_events (
			id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			type          VARCHAR(32)     NOT NULL,
			chain         VARCHAR(32)     NOT NULL,
			contract      CHAR(42)        NOT NULL,
			block_number  BIGINT UNSIGNED NOT NULL,
			block_hash    CHAR(66)        NOT NULL,
			tx_hash       CHAR(66)        NOT NULL,
			log_index     INT UNSIGNED    NOT NULL,
			from_addr     CHAR(42)        NOT NULL,
			to_addr       CHAR(42)        NOT NULL,
			value         VARCHAR(78)     NOT NULL,
			ts            DATETIME        NOT NULL,
			UNIQUE KEY uniq_log (tx_hash, log_index),
			KEY idx_block (block_number),
			KEY idx_contract (contract)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS listener_state (
			chain                VARCHAR(32)     NOT NULL PRIMARY KEY,
			last_processed_block BIGINT UNSIGNED NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *MySQL) LastProcessedBlock(ctx context.Context) (uint64, bool, error) {
	var n uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT last_processed_block FROM listener_state WHERE chain = ?`, s.chain,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query last block: %w", err)
	}
	return n, true, nil
}

func (s *MySQL) SaveEvents(ctx context.Context, blockNumber uint64, _ string, events []model.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交则 Rollback 为空操作

	const insert = `INSERT IGNORE INTO chain_events
		(type, chain, contract, block_number, block_hash, tx_hash, log_index, from_addr, to_addr, value, ts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, e := range events {
		if _, err := tx.ExecContext(ctx, insert,
			e.Type, e.Chain, e.Contract, e.BlockNumber, e.BlockHash,
			e.TxHash, e.LogIndex, e.From, e.To, e.Value, e.Timestamp,
		); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}

	// 推进进度:仅在更高区块时前移(GREATEST 防止并发/乱序回退)。
	const upsert = `INSERT INTO listener_state (chain, last_processed_block)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_processed_block = GREATEST(last_processed_block, VALUES(last_processed_block))`
	if _, err := tx.ExecContext(ctx, upsert, s.chain, blockNumber); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *MySQL) RevertFrom(ctx context.Context, from uint64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM chain_events WHERE block_number >= ?`, from)
	if err != nil {
		return 0, fmt.Errorf("delete events: %w", err)
	}
	affected, _ := res.RowsAffected()

	var newLast uint64
	if from > 0 {
		newLast = from - 1
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO listener_state (chain, last_processed_block) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE last_processed_block = VALUES(last_processed_block)`,
		s.chain, newLast,
	); err != nil {
		return 0, fmt.Errorf("reset state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return int(affected), nil
}

func (s *MySQL) Events(ctx context.Context) ([]model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT type, chain, contract, block_number, block_hash, tx_hash, log_index, from_addr, to_addr, value, ts
		 FROM chain_events ORDER BY block_number, log_index`)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var out []model.Event
	for rows.Next() {
		var e model.Event
		if err := rows.Scan(
			&e.Type, &e.Chain, &e.Contract, &e.BlockNumber, &e.BlockHash,
			&e.TxHash, &e.LogIndex, &e.From, &e.To, &e.Value, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *MySQL) Stats(ctx context.Context) Stats {
	var st Stats
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chain_events`).Scan(&st.TotalEvents)
	_ = s.db.QueryRowContext(ctx,
		`SELECT last_processed_block FROM listener_state WHERE chain = ?`, s.chain,
	).Scan(&st.LastProcessedBlock)
	return st
}

// Close 释放底层连接池。
func (s *MySQL) Close() error { return s.db.Close() }
