package convert

var execConverts = make(map[string]CreateFunc)

// CreateFunc create ExecConvert func point
type CreateFunc func(title, symbol string, supports []string) ExecConvert

// RegisterConvert 注册转换器（各转换器包在 init() 里调用）
func RegisterConvert(name string, create CreateFunc) {
	execConverts[name] = create
}

// LoadConvert 查找转换器
func LoadConvert(name string) (driver CreateFunc, err error) {
	c, ok := execConverts[name]
	if !ok {
		return nil, nil
	}

	return c, nil
}
