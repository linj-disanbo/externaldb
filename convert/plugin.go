package convert

import "github.com/33cn/externaldb/lib/schema"

// DBSaver DBSaver
// nolint
type DBSaver interface {
	InitDB(schema.DBCreator) error
}

// ExecConvert with db part
// 下阶段再分开， 可以配置不同的存储
type ExecConvert interface {
	Convert
	DBSaver
}
