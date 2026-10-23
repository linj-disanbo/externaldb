package exchange

import (
	"testing"

	converter "github.com/33cn/externaldb/convert"
	"github.com/33cn/externaldb/lib/schema"
	"github.com/33cn/externaldb/lib/util"
	"github.com/stretchr/testify/assert"
)

// 原测试文件是从 evmxgo 拷过来的（fixture 是 evmxgo 交易、断言的是 evmxgo 文档），
// 对本包没有意义；这里保留注册与“非 exchange 交易只写 transaction 记录”两条最小断言。
// 有真实 exchange 交易样例后再补具体单据（挂单/成交/撤单）的展开断言。
func Test_Register(t *testing.T) {
	schema.SetVersion(7)
	create, err := converter.LoadConvert("exchange")
	assert.NoError(t, err)
	assert.NotNil(t, create, "exchange 转换器应已注册")
	assert.NotNil(t, create("", "bty", nil))
}

func Test_ConvertTx_NotExchangeTx(t *testing.T) {
	convert := NewConvert("", "bty", nil)
	schema.SetVersion(7)

	// evmxgo 执行器的普通交易：payload 解不出 ExchangeAction，只落 transaction 记录
	blockByte := "0ad80212206dda19167cfbc53b96aa92cce9e40b44dfee8a1f233f9beff476c5457e27ec6b1a209ae30e0e1107e44760a950f7fcd7e2747b71de8cb8592b620ce50a23334cf81d222014521b0d5b500fae78844f3b087537be25dffac90990f062dc3cd267559130e828e80130fcdbed8b063abb010a0665766d78676f120e300d2a0a0a03444f471080ade2041a6d0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a463044022020b6183c4d41948894832b43c6b3feeaed3a691df94fd69b071d973ae144c8b40220537e9ebc4dfdcf0c69ae69a3c47e76a623296e03aa3c8214478b44922761ec2628f4dced8b063095f99b87c5abcdec7b3a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f80162205097215a432617350961a9a955187396dd33974399269d92ba6a1bef14057d7068e801127d08021a1f08c402121a0a0b0a03444f47188099a48a01120b0a03444f471880ecc185011a58080f12540a2810e0c9bf092221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152122810e09cdd042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152"
	env, err := util.GenEnv(blockByte, 0)
	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, schema.OpAdd)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "transaction", records[0].Index())
}
