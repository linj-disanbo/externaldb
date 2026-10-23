package proof

import (
	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/convert/proof/api"
	"github.com/33cn/externaldb/convert/proof/dao"
	"github.com/33cn/externaldb/convert/proof/proofdb"
	"github.com/33cn/externaldb/convert/proof/service"
	proofconfig "github.com/33cn/externaldb/convert/proof_config"
	"github.com/33cn/externaldb/lib/chaincfg"
	"github.com/33cn/externaldb/lib/schema"
)

// 本文件将做成插件需要的接口和函数都提到这个文件, 作为参考, 方便写文档, 也方便其他人实现新的插件
// 实现插件需要至少实现下面的内容
// 1. 注册插件
// 2. 注册插件需要的插件创建函数
// 3. InitDB 创建db
// 4. ConvertTx 展开数据
// 5. SetDB 插件需要访问数据库 (可选)

// 1注册插件, NewConvert函数需要满足接口 converter.ExecConvert
func init() {
	converter.RegisterConvert(api.AddProof, NewConvert)
	converter.RegisterConvert(api.MainAddProof, NewConvert)
	converter.RegisterConvert(api.TemplateTx, NewConvert)
	converter.RegisterConvert(api.MainTemplateTx, NewConvert)
	converter.RegisterConvert(api.DeleteTx, NewConvert)
	converter.RegisterConvert(api.MainDeleteTx, NewConvert)
	converter.RegisterConvert(api.RecoverTx, NewConvert)
	converter.RegisterConvert(api.MainRecoverTx, NewConvert)
}

// NewConvert 2注册插件需要的插件创建函数
func NewConvert(paraTitle, symbol string, supports []string) converter.ExecConvert {
	e := &service.ProofConvert{OrgTitle: paraTitle,
		Symbol: symbol, Name: api.AddProof, Title: chaincfg.CalcParaTitle(paraTitle)}
	e.RecordGen = service.NewRecordGen(proofdb.ProofDBX, proofdb.ProofTableX, proofdb.LogDBX, proofdb.LogTableX, proofdb.TemplateDBX, proofdb.TemplateTableX, proofdb.ProofUpdateDBX, proofdb.ProofUpdateTableX)
	return &Convert{convert: e}
}

// Convert ProofConvert
type Convert struct {
	Title   string
	Symbol  string
	Name    string
	convert *service.ProofConvert
	// wrapDB   converter.WrapDB
	// proofDB  proofdb.IProofDB
	// configDB proofconfig.PrivilegeDB
}

// InitDB 创建db (插件满足接口 converter.ExecConvert)
func (c *Convert) InitDB(cli schema.DBCreator) error {
	return dao.InitDB(cli, proofdb.ProofDBX, proofdb.ProofTableX, proofdb.LogDBX, proofdb.LogTableX, proofdb.TemplateDBX, proofdb.TemplateTableX, proofdb.ProofUpdateDBX, proofdb.ProofUpdateTableX)
}

// ConvertTx 展开数据 (插件满足接口 converter.ExecConvert)
func (c *Convert) ConvertTx(env *converter.TxEnv, op int) ([]schema.Record, error) {
	return c.convert.ConvertTx(env, op)
}

// SetDB SetDB (非必要: 插件需要访问数据库时需要实现, 一般使用于虚拟插件数据展开在es上包含执行逻辑)
func (c *Convert) SetDB(db converter.WrapDB) error {
	configDB := proofconfig.NewConfigDB(db)
	proofDB := dao.NewProofDB(db)
	return c.convert.SetDB(proofDB, configDB)
}
