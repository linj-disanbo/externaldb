package service

import (
	"github.com/33cn/chain33/common"
	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/convert/proof/api"
	"github.com/33cn/externaldb/convert/proof/model"
	"github.com/33cn/externaldb/convert/proof/proofdb"
	chaincfg "github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/schema"
)

type deleter struct {
	env    *converter.TxEnv
	arg    *api.DeleteProof
	txHash string
}

func newDeleter(env *converter.TxEnv, hash string, arg *api.DeleteProof) *deleter {
	return &deleter{env: env, txHash: hash, arg: arg}
}

func (d *deleter) del(p *model.Proof, ps []*model.Proof, op int, gen proofdb.IProofRecord) ([]schema.Record, error) {
	tx := d.env.Block.Block.Txs[d.env.TxIndex]
	from := chaincfg.AddressConvert(tx.From())
	hash := common.ToHex(tx.Hash())

	var records []schema.Record

	mp := newProof(p)
	ret := mp.del(op, hash, d.arg.Note, d.arg.Force)
	if !ret {
		// 强制删除, 不能回滚
		return nil, nil
	}

	op2 := schema.OpUpdate
	if d.arg.Force {
		op2 = schema.OpDel
	}
	id := p.Proof["proof_id"].(string)
	r1 := gen.Proof((*model.Proof)(mp), id, op2)
	records = append(records, r1)

	for _, p2 := range ps {
		mp := newProof(p2)
		mp.del(op, hash, d.arg.Note, d.arg.Force)
		id := p2.Proof["proof_id"].(string)
		r2 := gen.Proof((*model.Proof)(mp), id, op2)
		records = append(records, r2)
	}

	ml := newProofLog(hash, from)
	ml.setBlock(d.env.BlockHash, d.env.Block.Block.BlockTime, d.env.Block.Block.Height, d.env.TxIndex)
	ml.del(op, p, d.arg)
	op3 := schema.OpAdd
	if op == schema.SeqTypeDel {
		op3 = schema.OpDel
	}
	r3 := gen.Log((*model.Log)(ml), hash, op3)
	records = append(records, r3)

	return records, nil
}
