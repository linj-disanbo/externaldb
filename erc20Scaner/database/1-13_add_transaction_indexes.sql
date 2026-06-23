-- v1.13: 新增 ETH 通用交易查询索引
-- 背景：支持 /evmapi/transactions 通用查询接口，按合约地址 + 区块范围筛选
-- 日期：2026-06-23

-- 复合索引：按 to_address 查询并按 block_number 排序
ALTER TABLE transactions ADD INDEX idx_to_block (to_address, block_number);

-- 复合索引：按 from_address 查询并按 block_number 排序
ALTER TABLE transactions ADD INDEX idx_from_block (from_address, block_number);
