package convert

import (
	"context"
	"time"

	"github.com/33cn/chain33/common"
	"github.com/33cn/chain33/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// maxBatchSize 单批最大 seq 数：chain33 GetBlockSequences 要求 End-Start < MaxBlockCountPerTime(1000)
	maxBatchSize = 999
	// grpcMaxRecv 大区块（含 receipts）防超时
	grpcMaxRecv = 100 * 1024 * 1024
	// rpcTimeout 单次拉取超时
	rpcTimeout = 60 * time.Second
)

// SeqBlock 一个 seq 的区块数据：来源层的统一输出，喂给 ModuleConvert.SingleDealBlock。
type SeqBlock struct {
	Seq    int64
	Height int64
	Hash   string
	Type   int // 1=Add, 2=Del（同 db.SeqTypeAdd/SeqTypeDel）
	Detail *types.BlockDetail
}

// SeqSource 序列来源（可替换：备选 height+hash 方案只替换这一层的实现）
type SeqSource interface {
	// LastSeq 节点已记录的最大 seq
	LastSeq() (int64, error)
	// Batch 拉取 (start, end] 的区块，含 del seq；顺序即 seq 递增顺序
	Batch(start, end int64) ([]SeqBlock, error)
}

// seqClient 只声明用到的 gRPC 方法，便于单测注入 fake（types.Chain33Client 天然满足）
type seqClient interface {
	GetLastBlockSequence(ctx context.Context, in *types.ReqNil, opts ...grpc.CallOption) (*types.Int64, error)
	GetBlockSequences(ctx context.Context, in *types.ReqBlocks, opts ...grpc.CallOption) (*types.BlockSequences, error)
	GetBlockByHashes(ctx context.Context, in *types.ReqHashes, opts ...grpc.CallOption) (*types.BlockDetails, error)
}

// GRPCSource 通过 chain33 gRPC 按 seq 拉块
type GRPCSource struct {
	cli  seqClient
	conn *grpc.ClientConn
}

// NewGRPCSource 连接节点
func NewGRPCSource(addr string) (*GRPCSource, error) {
	if addr == "" {
		return nil, errors.New("seq_source: empty grpc address")
	}
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(grpcMaxRecv)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "seq_source: grpc dial")
	}
	return &GRPCSource{cli: types.NewChain33Client(conn), conn: conn}, nil
}

// Close 关闭连接
func (s *GRPCSource) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// LastSeq 节点已记录的最大 seq（节点的 seq 日志包含 Add/Del 两种）
func (s *GRPCSource) LastSeq() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	reply, err := s.cli.GetLastBlockSequence(ctx, &types.ReqNil{})
	if err != nil {
		return 0, errors.Wrap(err, "seq_source: GetLastBlockSequence")
	}
	return reply.GetData(), nil
}

// Batch 拉取 (start, end] 的区块。
//
// 两步：GetBlockSequences 拿 (hash,type)，GetBlockByHashes 拿完整 detail。
// 注意 chain33 的 BlockSequence **没有 seq 号字段**（只有 Hash/Type），只能按位置推号，
// 所以必须校验条数：数量不足时若不是"请求越过链尖被截断"，就说明 seq 日志有缺号 → 报错，
// 不允许像 convertV2 那样取 minLen 继续（会整体错位）。
func (s *GRPCSource) Batch(start, end int64) ([]SeqBlock, error) {
	if end < start {
		return nil, nil
	}
	if end-start+1 > maxBatchSize {
		return nil, errors.Errorf("seq_source: batch too large: %d (max %d)", end-start+1, maxBatchSize)
	}
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	seqReply, err := s.cli.GetBlockSequences(ctx, &types.ReqBlocks{Start: start, End: end})
	if err != nil {
		return nil, errors.Wrap(err, "seq_source: GetBlockSequences")
	}
	items := seqReply.GetItems()
	if len(items) == 0 {
		return nil, nil
	}
	need := end - start + 1
	if int64(len(items)) < need {
		// 正常情况：请求越过链尖，GetBlockSequences 会把 end 截到 blockLastSeq
		last, err := s.LastSeq()
		if err != nil {
			return nil, err
		}
		if start+int64(len(items))-1 < last {
			// 否则是中间缺号：定位第一个缺失的 seq 后报错（交人工处理，不静默跳过）
			miss, err := s.firstMissingSeq(ctx, start, end)
			if err != nil {
				return nil, err
			}
			return nil, errors.Errorf("seq_source: seq %d missing in node sequence log (range %d-%d)", miss, start, end)
		}
		end = start + int64(len(items)) - 1
	}

	hashes := make([][]byte, 0, len(items))
	for _, it := range items {
		hashes = append(hashes, it.GetHash())
	}
	detailReply, err := s.cli.GetBlockByHashes(ctx, &types.ReqHashes{Hashes: hashes})
	if err != nil {
		return nil, errors.Wrap(err, "seq_source: GetBlockByHashes")
	}
	details := detailReply.GetItems()
	if len(details) != len(items) {
		return nil, errors.Errorf("seq_source: seq/detail count mismatch: %d/%d (range %d-%d)",
			len(items), len(details), start, end)
	}

	out := make([]SeqBlock, 0, len(items))
	for i, it := range items {
		seq := start + int64(i)
		d := details[i]
		if d == nil || d.Block == nil {
			return nil, errors.Errorf("seq_source: empty block detail at seq %d", seq)
		}
		out = append(out, SeqBlock{
			Seq:    seq,
			Height: d.Block.GetHeight(),
			Hash:   common.ToHex(it.GetHash()),
			Type:   int(it.GetType()),
			Detail: d,
		})
	}
	return out, nil
}

// firstMissingSeq 在 [start, end] 内二分定位第一个缺失的 seq。
// 只在"条数不足且不是被链尖截断"时调用（这种情况下窗口都在链尖以内，不会被截断）。
func (s *GRPCSource) firstMissingSeq(ctx context.Context, start, end int64) (int64, error) {
	lo, hi := start, end
	for lo < hi {
		mid := (lo + hi) / 2
		full, err := s.windowFull(ctx, lo, mid)
		if err != nil {
			return 0, err
		}
		if full {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

// windowFull [start, end] 内的 seq 是否齐全
func (s *GRPCSource) windowFull(ctx context.Context, start, end int64) (bool, error) {
	reply, err := s.cli.GetBlockSequences(ctx, &types.ReqBlocks{Start: start, End: end})
	if err != nil {
		return false, errors.Wrap(err, "seq_source: GetBlockSequences(window)")
	}
	return int64(len(reply.GetItems())) == end-start+1, nil
}
