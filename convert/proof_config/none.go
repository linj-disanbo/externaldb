package proofconfig

import (
	converter "github.com/33cn/externaldb/convert"
)

type None struct {
	*configDB
}

// IsHaveProofPermission check Permission
func (n *None) IsHaveProofPermission(_ string) bool {
	return true
}

// IsHaveDelProofPermission check  DelProofPermission
func (n *None) IsHaveDelProofPermission(_, _, _ string) bool {
	return true
}

func (n *None) GetOrganizationName(addr string) (string, error) {
	if n.configDB != nil && n.configDB.db != nil {
		m, err := n.configDB.GetMember(addr)
		if err == nil {
			return m.Organization, nil
		}
	}
	return "system", nil
}

// GetMember 关闭权限校验时也要能正确回答“记录是否存在”：
// 直接返回空 Member 会让 AddMember 误判为“member exists”，导致成员记录写不进去。
func (n *None) GetMember(addr string) (*Member, error) {
	if n.configDB != nil && n.configDB.db != nil {
		return n.configDB.GetMember(addr)
	}
	return nil, converter.ErrDBNotFound
}
