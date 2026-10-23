package converts

import (
	"github.com/33cn/externaldb/db"
)

var execConverts = make(map[string]CreateFunc)

// CreateFunc create ExecConvert func point
type CreateFunc func(title, symbol string, supports []string) db.ExecConvert

// Register Register
func Register(name string, create CreateFunc) {
	execConverts[name] = create
}

// Load find exec
func Load(name string) (driver CreateFunc, err error) {
	c, ok := execConverts[name]
	if !ok {
		return nil, nil
	}

	return c, nil
}
