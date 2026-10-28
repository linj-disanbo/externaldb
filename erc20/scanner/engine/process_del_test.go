package engine

import (
	"testing"

	chain33types "github.com/33cn/chain33/types"

	"github.com/33cn/externaldb/lib/esdoc/block"
)

// del seq 的原高度只能从节点带回的明细里取（DelBlock 只删 height→hash，块体按 hash 仍在）；
// 取不到时返回 0，调用方跳过清理、交给后续同高度 add 的按块替换。
func TestDelBlockHeight(t *testing.T) {
	withHeight := &block.Seq{
		Type:        2,
		BlockDetail: chain33types.Encode(&chain33types.BlockDetail{Block: &chain33types.Block{Height: 48066968}}),
	}
	if got := delBlockHeight(withHeight); got != 48066968 {
		t.Fatalf("delBlockHeight = %d, want 48066968", got)
	}

	cases := []struct {
		name string
		seq  *block.Seq
	}{
		{"nil seq", nil},
		{"无明细", &block.Seq{Type: 2}},
		{"空明细（节点取不到块体时的 new(BlockDetail)）", &block.Seq{Type: 2, BlockDetail: chain33types.Encode(&chain33types.BlockDetail{})}},
		{"Block 为 nil", &block.Seq{Type: 2, BlockDetail: chain33types.Encode(&chain33types.BlockDetail{Receipts: nil})}},
		{"高度为 0", &block.Seq{Type: 2, BlockDetail: chain33types.Encode(&chain33types.BlockDetail{Block: &chain33types.Block{Height: 0}})}},
		{"非法明细", &block.Seq{Type: 2, BlockDetail: []byte{0xff, 0xff, 0xff, 0xff}}},
	}
	for _, c := range cases {
		if got := delBlockHeight(c.seq); got != 0 {
			t.Errorf("%s: delBlockHeight = %d, want 0", c.name, got)
		}
	}
}
