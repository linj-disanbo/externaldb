package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/33cn/externaldb/converts"
	"github.com/33cn/externaldb/stat"
)

// 守门测试：打包配置 (config/externaldb.toml) 里列出的 exec / stat 必须都能注册上。
// 背景：alter-config.sh 在 browser/common 模式下会把 ======start/end====== 段里的
// #[[convert.data]] 放开，若二进制漏 import 对应转换器包，NewApp 会
// panic("exec convert not exist: xxx")（例如曾经漏掉的 coinsx / pos33）。
// 这里连**注释掉的** exec 也一起校验，因为运行时可能被放开。
func TestConfiguredExecsAndStatsRegistered(t *testing.T) {
	b, err := os.ReadFile("../../config/externaldb.toml")
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	cfg := string(b)

	execRe := regexp.MustCompile(`(?m)^[ \t]*#?[ \t]*exec[ \t]*=[ \t]*"([^"]+)"`)
	found := false
	for _, m := range execRe.FindAllStringSubmatch(cfg, -1) {
		found = true
		name := m[1]
		d, err := converts.Load(name)
		assert.NoError(t, err)
		assert.NotNil(t, d, "exec convert 未注册（二进制漏 import 包）: %s", name)
	}
	assert.True(t, found, "配置里没解析到 exec，正则或路径需要检查")

	statRe := regexp.MustCompile(`(?m)^[ \t]*#?[ \t]*stat[ \t]*=[ \t]*"([^"]+)"`)
	for _, m := range statRe.FindAllStringSubmatch(cfg, -1) {
		name := m[1]
		d, err := stat.Load(name)
		assert.NoError(t, err)
		assert.NotNil(t, d, "stat 未注册（二进制漏 import 包）: %s", name)
	}
}
