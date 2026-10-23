package parser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
	// 依赖 swag 生成的输入（make swag 会把 swagger.json 放到 rpc/docs 下），
	// 仓库里没有这份 fixture 时跳过，而不是 panic。
	filePath := "../../testdata/swagger.json"
	if _, err := os.Stat(filePath); err != nil {
		t.Skipf("跳过：缺少输入文件 %s（先跑 make swag 生成）", filePath)
	}

	p, err := NewParser(filePath)
	assert.NoError(t, err)
	t.Logf("%+v", p)
	t.Logf(p.BuildTitle("存证、溯源后台管理模块接口文档"))
	t.Logf(p.BuildOverview())
	t.Logf(p.BuildDetail())

	outPath := "../../testdata/api.md"
	f, err := os.Create(outPath)
	assert.NoError(t, err)
	defer f.Close()

	_, _ = f.WriteString(p.BuildTitle("存证、溯源后台管理模块接口文档"))
	_, _ = f.WriteString(p.BuildOverview())
	_, _ = f.WriteString(p.BuildDetail())
}
