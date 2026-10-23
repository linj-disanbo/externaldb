package service

import (
	"github.com/33cn/externaldb/convert/proof/model"
	"github.com/33cn/externaldb/lib/schema"
)

type proof model.Proof

func newProof(p *model.Proof) *proof {
	return (*proof)(p)
}

func (p *proof) del(op int, hash, note string, force bool) bool {
	if op == schema.SeqTypeAdd {
		p.Proof["proof_deleted"] = hash
		p.Proof["proof_deleted_note"] = note
		p.Proof["proof_deleted_flag"] = true
	} else {
		p.Proof["proof_deleted"] = ""
		p.Proof["proof_deleted_note"] = ""
		p.Proof["proof_deleted_flag"] = false
	}
	if force && op == schema.SeqTypeDel {
		return false
	}
	return true
}

func (p *proof) recover(op int) {
	if op == schema.SeqTypeDel {
		p.Proof["proof_deleted_flag"] = true
	} else {
		p.Proof["proof_deleted_flag"] = false
	}
}
