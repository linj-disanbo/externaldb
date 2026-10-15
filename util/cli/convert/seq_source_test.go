package convert

import (
	"context"
	"testing"

	"github.com/33cn/chain33/common"
	"github.com/33cn/chain33/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/33cn/externaldb/db"
)

// fakeSeqClient 按 seq 表模拟节点：hashes[seq] 存在即该 seq 有区块
type fakeSeqClient struct {
	last         int64
	hashes       map[int64][]byte
	hashType     map[int64]int64
	hashHeight   map[string]int64
	missingBody  map[string]bool // 该 hash 的块体取不到（返回空 BlockDetail）
	detailLimit  int             // >0 时模拟 MaxBlockSizePerTime 提前返回（只给前 N 项）
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
		if f.detailLimit > 0 && len(out.Items) >= f.detailLimit {
			break // 模拟 MaxBlockSizePerTime：返回前缀
		}
		if f.missingBody[string(h)] {
			out.Items = append(out.Items, &types.BlockDetail{}) // 块体缺失：空 detail
			continue
		}
		out.Items = append(out.Items, &types.BlockDetail{Block: &types.Block{Height: f.hashHeight[string(h)]}})
	}
	return &out, nil
}

// 造 5,6,7 三个 seq，高度 100,101,102
func newFake() *fakeSeqClient {
	f := &fakeSeqClient{last: 7, hashes: map[int64][]byte{}, hashType: map[int64]int64{}, hashHeight: map[string]int64{}, missingBody: map[string]bool{}}
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

func TestGRPCSourceBatchTruncatedDetails(t *testing.T) {
	f := newFake()
	f.detailLimit = 2 // 模拟 MaxBlockSizePerTime：details 只返回前缀
	s := &GRPCSource{cli: f}

	bs, err := s.Batch(5, 7)
	assert.NoError(t, err, "细节前缀截断属于正常情况，按取到的长度处理")
	if assert.Len(t, bs, 2) {
		assert.Equal(t, int64(5), bs[0].Seq)
		assert.Equal(t, int64(6), bs[1].Seq)
		assert.Equal(t, int64(100), bs[0].Height)
	}
}

func TestGRPCSourceBatchDelWithoutDetail(t *testing.T) {
	f := newFake()
	f.hashType[7] = types.DelBlock
	f.missingBody[string([]byte{7})] = true // 块体已被 prune
	s := &GRPCSource{cli: f}

	bs, err := s.Batch(7, 7)
	assert.NoError(t, err, "del seq 缺块体允许 Detail 为 nil，交调用方兜底")
	if assert.Len(t, bs, 1) {
		assert.Equal(t, db.SeqTypeDel, bs[0].Type)
		assert.Nil(t, bs[0].Detail)
	}
}

func TestGRPCSourceBatchAddWithoutDetail(t *testing.T) {
	f := newFake()
	f.missingBody[string([]byte{6})] = true // add seq 缺块体 = 数据问题
	s := &GRPCSource{cli: f}

	_, err := s.Batch(5, 6)
	if assert.Error(t, err) {
		assert.True(t, errors.Is(err, ErrSeqHole))
		assert.Contains(t, err.Error(), "missing block detail at seq 6")
	}
}

func TestGRPCSourceBlockByHash(t *testing.T) {
	f := newFake()
	s := &GRPCSource{cli: f}

	d, err := s.BlockByHash(common.ToHex([]byte{5}))
	assert.NoError(t, err)
	if assert.NotNil(t, d) {
		assert.Equal(t, int64(100), d.Block.Height)
	}

	// 块体不存在 → (nil, nil)，不报错
	f.missingBody[string([]byte{6})] = true
	d, err = s.BlockByHash(common.ToHex([]byte{6}))
	assert.NoError(t, err)
	assert.Nil(t, d)

	// 空 hash 直接返回
	d, err = s.BlockByHash("")
	assert.NoError(t, err)
	assert.Nil(t, d)
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
