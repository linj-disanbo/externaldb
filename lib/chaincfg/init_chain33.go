package chaincfg

import (
	_ "embed"
	"fmt"

	_ "github.com/33cn/chain33/system" // chain system init
	"github.com/33cn/chain33/types"
	_ "github.com/33cn/plugin/plugin" // chain plugin init
)

// types/config L100 初始化， 需要手动调用， 不能自动
// 1. chain33 更新配置的实现后， 插件需要手动调用生效
// 2. 用于不同的链，插件不同加载的配置不同
// 3. 提供一个全的配置， 用对应链的配置来覆盖默认的配置， 再用这个配置来初始化
// 4. meger config 不可用， 只能用于合并 chain33配置和plugin的配置(合并的项不能重复)， 和这里覆盖不一样
// 所以要在这里实现一个

// chain33V172 内嵌的完整 chain33 配置（v1.72.3 + plugin v1.74.0 的全部 fork）。
// 之前内嵌的是 v1.65 配置，v1.72 起 fork 严格校验会直接 panic（缺 system/exec fork）。
//
//go:embed chain33_v172.toml
var chain33V172 string

// InitChain33 初始化chain33
//
// 直接用完整配置（内嵌 v1.72 完整配置，或调用方给的配置文件），**不再**与旧的
// Chain33DefaultConfig 合并：那份基础配置里有已下线的 fork（如 pos33 ForkReward15），
// v1.72 的严格校验会报 "exec fork not exist in code"。
func InitChain33(title, symbol, configFile string) {
	configString := configV172(title, symbol, chain33V172)
	if configFile != "" {
		configString = types.ReadFile(configFile)
	}
	types.NewChain33Config(configString)
}

func configV172(title, symbol, defaultConfig string) string {
	titleStr := fmt.Sprintf(Chain33TitleFmt, title)
	symbolStr := fmt.Sprintf(Chain33CoinSymbol, symbol)
	configString := titleStr + symbolStr + defaultConfig
	return configString
}
