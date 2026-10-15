package convert

import (
	"testing"

	"github.com/33cn/chain33/types"
	"github.com/stretchr/testify/assert"
)

func d(hash string, height int64) (string, *types.BlockDetail) {
	return hash, &types.BlockDetail{Block: &types.Block{Height: height}}
}

func TestDetailCachePutGet(t *testing.T) {
	c := NewDetailCache(3)

	h, det := d("a", 1)
	c.Put(h, det)
	got := c.Get("a")
	if assert.NotNil(t, got) {
		assert.Equal(t, int64(1), got.Block.Height)
	}
	assert.Nil(t, c.Get("nope"))
	assert.Nil(t, c.Get(""))
	assert.Equal(t, 1, c.Len())

	// 空 hash / nil detail 忽略
	c.Put("", det)
	c.Put("b", nil)
	assert.Equal(t, 1, c.Len())

	// 同 hash 覆盖不增长
	_, det2 := d("a", 2)
	c.Put("a", det2)
	assert.Equal(t, 1, c.Len())
	assert.Equal(t, int64(2), c.Get("a").Block.Height)
}

func TestDetailCacheEvictsOldest(t *testing.T) {
	c := NewDetailCache(3)
	c.Put("01", &types.BlockDetail{Block: &types.Block{Height: 1}})
	c.Put("02", &types.BlockDetail{Block: &types.Block{Height: 2}})
	c.Put("03", &types.BlockDetail{Block: &types.Block{Height: 3}})
	assert.Equal(t, 3, c.Len())

	// 第 4 个挤掉最旧的 01
	c.Put("04", &types.BlockDetail{Block: &types.Block{Height: 4}})
	assert.Equal(t, 3, c.Len())
	assert.Nil(t, c.Get("01"))
	for _, k := range []string{"02", "03", "04"} {
		assert.NotNil(t, c.Get(k), k)
	}

	// 继续轮转，顺序正确（FIFO）
	c.Put("05", &types.BlockDetail{Block: &types.Block{Height: 5}})
	assert.Nil(t, c.Get("02"))
	assert.NotNil(t, c.Get("05"))
	assert.Equal(t, 3, c.Len())
}

func TestDetailCacheDefaultSize(t *testing.T) {
	c := NewDetailCache(0)
	assert.Equal(t, DefaultDetailCacheSize, c.max)
	c2 := NewDetailCache(-1)
	assert.Equal(t, DefaultDetailCacheSize, c2.max)
}
