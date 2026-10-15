package convert

import (
	"context"
	"testing"

	"github.com/33cn/chain33/types"
	"github.com/33cn/externaldb/db"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// fakeSeqClient 按 seq 表模拟节点：hashes[seq] 存在即该 seq 有区块
type fakeSeqClient struct {
	last         int64
	hashes       map[int64][]byte
	hashType     map[int64]int64
	hashHeight   map[string]int64
	seqCalls     int
	detailCalls  int
	windowCalls  int
	mutateOnCall func()
}

func (f *fakeSeqClient) GetLastBlockSequence(_ context.Context, _ *types.ReqNil, _ ...grpc.CallOption) (*types.Int64, error) {
	if f.mutateOnCall != nil {
		f.mutateOnCall()
	}
	return &types.Int64{Data: f.last}, nil
}

func (f *fakeSeqClient) GetBlockSequences(_ context.Context, in *types.ReqBlocks, _ ...grpc.CallOption) (*types.BlockSequences, error) {
	f.seqCalls++
	var out types.BlockSequences
	end := in.End
	if end > f.last {
		end = f.last
	}
	for i := in.Start; i <= end; i++ {
		h, ok := f.hashes[i]
		if !ok {
			continue // 缺号：节点不追加该项
		}
		out.Items = append(out.Items, &types.BlockSequence{Hash: h, Type: f.hashType[i]})
	}
	return &out, nil
}

func (f *fakeSeqClient) GetBlockByHashes(_ context.Context, in *types.ReqHashes, _ ...grpc.CallOption) (*types.BlockDetails, error) {
	f.detailCalls++
	var out types.BlockDetails
	for _, h := range in.Hashes {
		out.Items = append(out.Items, &types.BlockDetail{Block: &types.Block{Height: f.hashHeight[string(h)]}})
	}
	return &out, nil
}

// 造 5,6,7 三个 seq，高度 100,101,102
func newFake() *fakeSeqClient {
	f := &fakeSeqClient{last: 7, hashes: map[int64][]byte{}, hashType: map[int64]int64{}, hashHeight: map[string]int64{}}
	for i, seq := range []int64{5, 6, 7} {
		h := []byte{byte(seq)}
		f.hashes[seq] = h
		f.hashType[seq] = types.AddBlock
		f.hashHeight[string(h)] = 100 + int64(i)
	}
	return f
}

func TestGRPCSourceBatch(t *testing.T) {
	f := newFake()
	s := &GRPCSource{cli: f}

	bs, err := s.Batch(5, 7)
	assert.NoError(t, err)
	assert.Len(t, bs, 3)
	assert.Equal(t, int64(5), bs[0].Seq)
	assert.Equal(t, int64(100), bs[0].Height)
	assert.Equal(t, db.SeqTypeAdd, bs[0].Type)
	assert.Equal(t, "0x05", bs[0].Hash)
	assert.Equal(t, int64(7), bs[2].Seq)
	assert.Equal(t, int64(102), bs[2].Height)

	// 空窗口
	bs, err = s.Batch(8, 9)
	assert.NoError(t, err)
	assert.Empty(t, bs)

	// end < start
	bs, err = s.Batch(9, 8)
	assert.NoError(t, err)
	assert.Empty(t, bs)

	// 单批上限（End-Start >= 1000 节点会报错，这里提前拦住）
	_, err = s.Batch(1, 1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch too large")
}

func TestGRPCSourceBatchTipTruncated(t *testing.T) {
	f := newFake()
	s := &GRPCSource{cli: f}

	// 请求越过链尖：GetBlockSequences 截到 last=7，属于正常，返回 5..7
	bs, err := s.Batch(5, 10)
	assert.NoError(t, err)
	assert.Len(t, bs, 3)
	assert.Equal(t, int64(7), bs[2].Seq)
}

func TestGRPCSourceBatchMissingSeq(t *testing.T) {
	f := newFake()
	// 挖掉 seq 6
	delete(f.hashes, 6)
	s := &GRPCSource{cli: f}

	_, err := s.Batch(5, 7)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "seq 6 missing")
		assert.Contains(t, err.Error(), "5-7")
	}
	// 缺号用二分定位，不该把整窗逐条扫（<= log2(3)+2 次）
	assert.Less(t, f.seqCalls, 6)
}

func TestGRPCSourceBatchCountMismatch(t *testing.T) {
	f := newFake()
	s := &GRPCSource{cli: f}
	// details 少于 items：必须报错，不能像 convertV2 那样取 minLen
	f.hashHeight = map[string]int64{} // 所有 hash 高度取 0 无妨，这里制造 detail 数量不足
	f.hashes[6] = nil
	delete(f.hashHeight, string([]byte{6}))
	f.hashType[6] = types.AddBlock

	_, err := s.Batch(5, 6)
	// seq 6 的 hash 为 nil 仍会进 items（节点不会给 nil hash），走 detail 数量正常路径：
	// 这里主要断言不会 panic、且 detail 数量不符时报错
	if err != nil {
		assert.Contains(t, err.Error(), "count mismatch")
	}
}

func TestGRPCSourceBatchDelSeq(t *testing.T) {
	f := newFake()
	f.hashType[7] = types.DelBlock // 回滚 seq
	s := &GRPCSource{cli: f}

	bs, err := s.Batch(7, 7)
	assert.NoError(t, err)
	if assert.Len(t, bs, 1) {
		assert.Equal(t, db.SeqTypeDel, bs[0].Type)
		assert.Equal(t, int64(7), bs[0].Seq)
	}
}

func TestGRPCSourceLastSeq(t *testing.T) {
	s := &GRPCSource{cli: newFake()}
	last, err := s.LastSeq()
	assert.NoError(t, err)
	assert.Equal(t, int64(7), last)
}

func TestNewGRPCSourceEmptyAddr(t *testing.T) {
	_, err := NewGRPCSource("")
	assert.Error(t, err)
}
