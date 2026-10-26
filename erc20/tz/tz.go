// Package tz 提供进程内固定的东八区位置，并把 IANA 时区库编进二进制。
//
// 本仓库二进制是静态编译（CGO_ENABLED=0），time.LoadLocation 只能读镜像里的
// /usr/share/zoneinfo；精简镜像/未装 tzdata 时会返回 (nil, err)，调用方若忽略
// 错误继续 t.In(nil)，就会 panic："time: missing Location in call to Time.In"。
// 这里 _ "time/tzdata" 把时区库内嵌（Go 1.15+，约 +450KB），彻底去掉对镜像的
// 依赖；即使内嵌数据异常也还有固定 +08:00 兜底，保证返回的 Location 永不为 nil。
package tz

import (
	"time"
	_ "time/tzdata" // 内嵌 IANA 时区库，使 LoadLocation 不依赖镜像的 zoneinfo
)

const name = "Asia/Shanghai"

// cst 东八区位置；初始化时加载一次（内嵌 tzdata 正常时走真实 IANA 规则）
var cst = load()

func load() *time.Location {
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	// 兜底：固定 +08:00（无反解时区名，仍可安全用于 Format/In）
	return time.FixedZone("CST", 8*60*60)
}

// CST 返回东八区（Asia/Shanghai）位置，永不返回 nil。
func CST() *time.Location { return cst }
