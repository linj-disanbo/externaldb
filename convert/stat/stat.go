package stat

import (
	"github.com/33cn/chain33/types"
	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/lib/schema"
)

// StatCreateFunc 创建统计器
type StatCreateFunc func(title, symbol string, genesis, coin int64) Stat

var statConverts = make(map[string]StatCreateFunc)

// Register 注册统计器（由各统计实现包在 init() 里调用）
func Register(name string, create StatCreateFunc) {
	statConverts[name] = create
}

// Load 查找统计器
func Load(name string) (driver StatCreateFunc, err error) {
	c, ok := statConverts[name]
	if !ok {
		return nil, nil
	}
	return c, nil
}

// Stat 统计
type Stat interface {
	converter.DBSaver
	Stat(detail *types.BlockDetail, op int) ([]schema.Record, error)
	Recover(client escli.ESClient, lastSeq int64) error
}

// HeightRecover 可选接口：按"已处理到的区块高度"恢复累计值。
//
// 为什么需要它：累计类统计（如 block_stat 的 prev）必须与**恢复点**对齐——
//   - 处理高度 H 的 Add：prev 必须是"累计到 H-1"的值；
//   - 处理高度 H 的 Del：prev 必须是"累计到 H"的值（再减去本块）。
//
// 只按 lastSeq 取"全局最新"的记录是不行的：崩溃重放（数据已写、marker 未写）时，
// 最新记录可能已经领先于游标，会把同一块重复累加（block_stat 类计数不是幂等的）。
type HeightRecover interface {
	// RecoverAt height>=0：恢复出"已处理完 height 高度"对应的累计值；
	// height<0：不设上界（等价旧的 Recover 行为）。
	RecoverAt(client escli.ESClient, height int64) error
}
