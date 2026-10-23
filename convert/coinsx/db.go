package coinsx

import (
	"encoding/json"

	"github.com/33cn/externaldb/lib/schema"
	coinTy "github.com/33cn/plugin/plugin/dapp/coinsx/types"
)

const (
	CoinsxManagerDBX    = "coins_manager"
	CoinsxManagerTableX = "coins_manager"
	DefaultType         = "_doc"
)

type Manager struct {
	ManagerStatus *coinTy.ManagerStatus `json:"manager_status"`
	HeightIndex   int64                 `json:"height_index"`
}

type Record struct {
	*schema.IKey
	*schema.Op
	Manager Manager
}

func (r *Record) Value() []byte {
	v, _ := json.Marshal(r.Manager)
	return v
}

func newCoinsxKey(flag coinTy.TransferFlag) *schema.IKey {
	return schema.NewIKey(CoinsxManagerDBX, CoinsxManagerTableX, string(flag))
}
