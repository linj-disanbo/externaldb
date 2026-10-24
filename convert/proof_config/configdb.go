package proofconfig

import (
	"encoding/json"

	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/lib/schema"
	lru "github.com/hashicorp/golang-lru"
)

var OpenAccessControl = false
var memberCache *lru.Cache
var orgCache *lru.Cache

func init() {
	var err error
	memberCache, err = lru.New(1024)
	if err != nil {
		panic(err)
	}
	orgCache, err = lru.New(1024)
	if err != nil {
		panic(err)
	}
}

// ConfigDB ConfigDB
type configDB struct {
	dbX, tableX       string
	orgDBX, orgTableX string
	delDBX, delTableX string
	db                converter.WrapDB
}

func InitOpenAccessControl(isOpen bool) {
	OpenAccessControl = isOpen
}

// NewConfigDB NewConfigDB
func NewConfigDB(db converter.WrapDB) ConfigDB {
	d := &configDB{
		db:        db,
		dbX:       DBX,
		tableX:    TableX,
		delDBX:    DeleteDBX,
		delTableX: DeleteTableX,
		orgDBX:    OrgDBX,
		orgTableX: OrgTableX,
	}
	if OpenAccessControl {
		return d
	}
	return &None{d}
}

func (db1 *configDB) GetMember(addr string) (*Member, error) {
	if c, ok := memberCache.Get(addr); ok {
		if m, ok := c.(*Member); ok {
			return m, nil
		}
		// 缓存里可能是别的类型（历史 bug：SetMember 存过 *dbMember、SetOrganization 存过 *dbOrganization），清掉重查
		memberCache.Remove(addr)
	}
	m1, err := db1.db.Get(db1.dbX, db1.tableX, MemberID(addr))
	if err != nil {
		return nil, err
	}
	var mem Member
	err = json.Unmarshal(*m1, &mem)
	if err != nil {
		return nil, err
	}
	memberCache.Add(addr, &mem)
	return &mem, nil
}

func (db1 *configDB) GetMemberDel(addr string, h, i int64) (*memberDel, error) {
	m1, err := db1.db.Get(db1.delDBX, db1.delTableX, memberDelID(addr, h, i))
	if err != nil {
		return nil, err
	}
	var mem memberDel
	err = json.Unmarshal(*m1, &mem)
	if err != nil {
		return nil, err
	}
	return &mem, nil
}

func (db1 *configDB) GetOrganization(org string) (*Organization, error) {
	if c, ok := orgCache.Get(org); ok {
		if o, ok := c.(*Organization); ok {
			return o, nil
		}
		orgCache.Remove(org)
	}
	m1, err := db1.db.Get(db1.orgDBX, db1.orgTableX, OrganizationID(org))
	if err != nil {
		return nil, err
	}
	var o Organization
	err = json.Unmarshal(*m1, &o)
	if err != nil {
		return nil, err
	}
	orgCache.Add(org, &o)
	return &o, nil
}

func (db1 *configDB) SetMember(addr string, m *dbMember) error {
	if m.OpType() == schema.OpDel {
		// 删除时同时失效缓存，否则删除后重新添加会被误判为“已存在”
		memberCache.Remove(addr)
	} else {
		memberCache.Add(addr, m.M)
	}
	return db1.db.Set(db1.dbX, db1.tableX, m.M.ID(), m)
}

func (db1 *configDB) SetMemberDel(addr string, m *dbMemberDel) error {
	return db1.db.Set(db1.delDBX, db1.delTableX, m.M.ID(), m)
}

func (db1 *configDB) SetOrganization(org string, o *dbOrganization) error {
	if o.OpType() == schema.OpDel {
		orgCache.Remove(org)
	} else {
		orgCache.Add(org, o.M)
	}
	return db1.db.Set(db1.orgDBX, db1.orgTableX, o.M.ID(), o)
}

// IsHaveProofPermission check Permission
func (db1 *configDB) IsHaveProofPermission(addr string) bool {
	_, err := db1.GetMember(addr)
	if err != nil {
		log.Error("get member failed", "err", err, "address", addr)
		return false
	}
	return true
}

// GetOrganizationName Get Organization by member
func (db1 *configDB) GetOrganizationName(addr string) (string, error) {
	m, err := db1.GetMember(addr)
	if err != nil {
		log.Error("get member failed", "err", err, "address", addr)
		return "", err
	}
	return m.Organization, nil
}

// IsHaveDelProofPermission check  DelProofPermission
func (db1 *configDB) IsHaveDelProofPermission(send, proofOrg, proofOwner string) bool {
	m, err := db1.GetMember(send)
	if err != nil {
		log.Error("get member failed", "err", err, "address", send)
		return false
	}
	return m.PrivilegeToProof(proofOrg, proofOwner)
}
