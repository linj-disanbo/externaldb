-- 扫块进度改为 append-only 的 seq marker（2026-10）
--
-- 背景：scan_progress 单行只存 height，del seq 无法识别、崩溃重放无法对账。
-- 新表一个 seq 一行，游标 = max(seq)；del seq 也写一行（type=2），游标不回退。
-- 老数据迁移：首次启动若 scan_seq_marker 为空，则以 scan_progress.last_block_number
-- 回退一次（alignScanStart 把 height 换成 seq），随后写第一条 marker。

CREATE TABLE IF NOT EXISTS scan_seq_marker (
    seq BIGINT UNSIGNED NOT NULL COMMENT 'chain33 seq（主键，append-only）',
    height BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '区块高度（del seq 可能为 0）',
    hash VARCHAR(66) NOT NULL DEFAULT '' COMMENT '区块哈希',
    type TINYINT NOT NULL DEFAULT 0 COMMENT '1=add 2=del（chain33 seq type）',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (seq),
    KEY idx_height (height)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='扫块 seq 进度标记';
