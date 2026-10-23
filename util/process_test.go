package util

import (
	"testing"

	"github.com/33cn/chain33/types"
	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/esdoc/block"
	"github.com/33cn/externaldb/lib/schema"
)

type fakeRecord struct {
	*schema.IKey
	*schema.Op
}

func (r *fakeRecord) Value() []byte { return []byte(`{}`) }

func rec(id string) schema.Record {
	return &fakeRecord{IKey: schema.NewIKey("account", "account", id), Op: schema.NewOp(schema.OpAdd)}
}

// fakeApp 只回固定的记录序列，用于验证 DealBlockDetail 的顺序语义
type fakeApp struct {
	records []schema.Record
}

func (f *fakeApp) ConvertBlock(_ *block.Seq, _ *types.BlockDetail) ([]schema.Record, error) {
	return f.records, nil
}

func (f *fakeApp) RecoverStats(_ escli.ESClient, _ int64) error { return nil }

func newMod() *ModuleConvert {
	return &ModuleConvert{Name: "test", AppConvert: &fakeApp{records: []schema.Record{rec("a"), rec("b"), rec("c")}}}
}

func ids(rs []schema.Record) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID())
	}
	return out
}

func detailWith(txs, receipts int) *types.BlockDetail {
	d := &types.BlockDetail{Block: &types.Block{Height: 10}}
	for i := 0; i < txs; i++ {
		d.Block.Txs = append(d.Block.Txs, &types.Transaction{})
	}
	for i := 0; i < receipts; i++ {
		d.Receipts = append(d.Receipts, &types.ReceiptData{})
	}
	return d
}

func TestDealBlockDetailReversesOnDel(t *testing.T) {
	mod := newMod()

	// Add：顺序保持
	rs, err := mod.DealBlockDetail(&block.Seq{Type: schema.SeqTypeAdd, SyncSeq: 1}, detailWith(1, 1))
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids(rs))

	// Del：必须逆序反做（回滚语义）
	rs, err = mod.DealBlockDetail(&block.Seq{Type: schema.SeqTypeDel, SyncSeq: 2}, detailWith(1, 1))
	assert.NoError(t, err)
	assert.Equal(t, []string{"c", "b", "a"}, ids(rs), "回滚必须逆序")
}

func TestDealBlockDetailGuards(t *testing.T) {
	mod := newMod()

	// nil blockSeq
	_, err := mod.DealBlockDetail(nil, detailWith(1, 1))
	assert.Error(t, err)

	// nil detail / nil block
	_, err = mod.DealBlockDetail(&block.Seq{Type: schema.SeqTypeAdd}, nil)
	assert.Error(t, err)
	_, err = mod.DealBlockDetail(&block.Seq{Type: schema.SeqTypeAdd}, &types.BlockDetail{})
	assert.Error(t, err)

	// Txs/Receipts 数量不一致：跳过（返回空记录、不报错），与旧路径一致
	rs, err := mod.DealBlockDetail(&block.Seq{Type: schema.SeqTypeAdd}, detailWith(2, 1))
	assert.NoError(t, err)
	assert.Nil(t, rs)
}

func TestSingleDealBlockUsesSamePath(t *testing.T) {
	mod := newMod()
	detail := detailWith(1, 1)
	encoded := types.Encode(detail)

	// 旧路径（sync_convert 用）：从 BlockDetail 字节解码后再处理
	rs, err := mod.SingleDealBlock(&block.Seq{Type: schema.SeqTypeAdd, BlockDetail: encoded})
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ids(rs))

	rs, err = mod.SingleDealBlock(&block.Seq{Type: schema.SeqTypeDel, BlockDetail: encoded})
	assert.NoError(t, err)
	assert.Equal(t, []string{"c", "b", "a"}, ids(rs))
}
