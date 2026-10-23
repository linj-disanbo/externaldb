package pipeline

import (
	"sync"

	"github.com/33cn/chain33/types"
)

// DefaultDetailCacheSize 默认缓存最近多少块 detail。
// 用于回滚：del seq 只带被回滚块的 hash，逆序反做需要该块的 detail；
// 节点 DelBlock 不删 block body（可用 GetBlockByHashes 兜底），但存在 chunk/prune 删块体的情况，
// 所以本地保留一个窗口。窗口要覆盖"停机期间 + 运行期"的回滚范围。
const DefaultDetailCacheSize = 1000

// DetailCache 最近 N 个区块 detail 的 FIFO 缓存（key = 区块 hash）
type DetailCache struct {
	mu   sync.Mutex
	max  int
	next int
	ring []string
	m    map[string]*types.BlockDetail
}

// NewDetailCache 创建缓存；size<=0 时用默认值
func NewDetailCache(size int) *DetailCache {
	if size <= 0 {
		size = DefaultDetailCacheSize
	}
	return &DetailCache{
		max:  size,
		ring: make([]string, 0, size),
		m:    make(map[string]*types.BlockDetail, size),
	}
}

// Put 写入 detail；hash 为空或 detail 为 nil 时忽略
func (c *DetailCache) Put(hash string, detail *types.BlockDetail) {
	if hash == "" || detail == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[hash]; ok {
		c.m[hash] = detail // 同 hash 覆盖，不占新槽位
		return
	}
	if len(c.ring) < c.max {
		c.ring = append(c.ring, hash)
	} else {
		evict := c.ring[c.next]
		delete(c.m, evict)
		c.ring[c.next] = hash
		c.next = (c.next + 1) % c.max
	}
	c.m[hash] = detail
}

// Get 取 detail，未命中返回 nil
func (c *DetailCache) Get(hash string) *types.BlockDetail {
	if hash == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[hash]
}

// Len 当前缓存的区块数
func (c *DetailCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
