-- MySQL schema(程序启动时会自动执行 CREATE TABLE IF NOT EXISTS,此文件供参考/手动建库)。
--
-- 建库示例:
--   CREATE DATABASE IF NOT EXISTS chain_indexer DEFAULT CHARSET utf8mb4;
--   mysql -u root -p chain_indexer < schema.sql

-- 链上事件表:唯一索引 (tx_hash, log_index) 保证幂等去重。
CREATE TABLE IF NOT EXISTS chain_events (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    type          VARCHAR(32)     NOT NULL COMMENT 'ERC20_Transfer / ERC721_Transfer',
    chain         VARCHAR(32)     NOT NULL,
    contract      CHAR(42)        NOT NULL,
    block_number  BIGINT UNSIGNED NOT NULL,
    block_hash    CHAR(66)        NOT NULL,
    tx_hash       CHAR(66)        NOT NULL,
    log_index     INT UNSIGNED    NOT NULL,
    from_addr     CHAR(42)        NOT NULL,
    to_addr       CHAR(42)        NOT NULL,
    value         VARCHAR(78)     NOT NULL COMMENT 'ERC20 金额 / ERC721 tokenId(uint256 最长 78 位)',
    ts            DATETIME        NOT NULL,
    UNIQUE KEY uniq_log (tx_hash, log_index),
    KEY idx_block (block_number),
    KEY idx_contract (contract)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 监听进度表:按链记录已处理的最高区块,支持断点续传与重组回滚。
CREATE TABLE IF NOT EXISTS listener_state (
    chain                VARCHAR(32)     NOT NULL PRIMARY KEY,
    last_processed_block BIGINT UNSIGNED NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
