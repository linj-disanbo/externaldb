package exchange

import (
	"testing"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/util"
	"github.com/stretchr/testify/assert"
)

func TestConvert_ConvertTx(t *testing.T) {
	convert := NewConvert("", "bty", nil)
	db.SetVersion(6)

	convertTokenBurn(t, convert)
	convertTokenMint(t, convert)
	convertTokenTransfer(t, convert)
	convertTokenTransferToExec(t, convert)
	convertTokenWithdraw(t, convert)
}

func convertTokenBurn(t *testing.T, convert db.ExecConvert) {
	blockByte := "0ad80212206dda19167cfbc53b96aa92cce9e40b44dfee8a1f233f9beff476c5457e27ec6b1a209ae30e0e1107e44760a950f7fcd7e2747b71de8cb8592b620ce50a23334cf81d222014521b0d5b500fae78844f3b087537be25dffac90990f062dc3cd267559130e828e80130fcdbed8b063abb010a0665766d78676f120e300d2a0a0a03444f471080ade2041a6d0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a463044022020b6183c4d41948894832b43c6b3feeaed3a691df94fd69b071d973ae144c8b40220537e9ebc4dfdcf0c69ae69a3c47e76a623296e03aa3c8214478b44922761ec2628f4dced8b063095f99b87c5abcdec7b3a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f80162205097215a432617350961a9a955187396dd33974399269d92ba6a1bef14057d7068e801127d08021a1f08c402121a0a0b0a03444f47188099a48a01120b0a03444f471880ecc185011a58080f12540a2810e0c9bf092221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152122810e09cdd042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152"
	index := int64(0)
	env, err := util.GenEnv(blockByte, index)

	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, db.OpAdd)
	assert.Nil(t, err)

	assert.Equal(t, 4, len(records))
	assert.Equal(t, "evmxgo_info/evmxgo_info/ABCDE", records[1].Key())
	assert.Equal(t, `{"amount":99999990000}`, string(records[1].Value()))
}

func convertTokenMint(t *testing.T, convert db.ExecConvert) {
	blockByte := "0ad80212200e4837706fa2cc2c22e2026d44aa57d9b8a729f24135478188c34f0bae95f2a31a207a56a55d3fd957086b49b0e6099682bfcc03446fd6f314bf15c2d722e322b4652220025ac14411e7305f42f5ddefbf65672c6dba6730e1c017c627e03afee82768a928e70130fbdbed8b063abb010a0665766d78676f120e300c220a0a03444f471080ade2041a6d0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a463044022035133e1830bbd25a796f1a53b5ebd82d1d820e3ce5b40aebcde81a6eec3f9e850220160809da7c37595ce993620df6209a62a55c11dad6f7a2e3fb8421aecf78356a28f3dced8b0630ecc190a9f6e88fec4d3a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f80162206dda19167cfbc53b96aa92cce9e40b44dfee8a1f233f9beff476c5457e27ec6b68e701127d08021a1f08c302121a0a0b0a03444f471880ecc18501120b0a03444f47188099a48a011a58080e12540a2810e09cdd042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152122810e0c9bf092221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152"
	index := int64(0)
	env, err := util.GenEnv(blockByte, index)
	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, db.OpAdd)
	assert.Nil(t, err)

	assert.Equal(t, 4, len(records))
	assert.Equal(t, "evmxgo_info/evmxgo_info/ABCDE", records[1].Key())
	assert.Equal(t, `{"amount":100000000000}`, string(records[1].Value()))
}

func convertTokenTransfer(t *testing.T, convert db.ExecConvert) {
	blockByte := "0afc0212205097215a432617350961a9a955187396dd33974399269d92ba6a1bef14057d701a2045c3da6d8b483196a1dbbf6909dae6ba81a442532c0778b9caa022582b1d4ebe222003510a4ea56988f0e36c27ce48dc03422b69f681a582667b82214233984ecdb028e90130fddbed8b063adf010a0665766d78676f123230040a2e0a03444f471080ade2042222313745567636745732487a45373354564236595851595468514a7861376b755a62381a6d0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a463044022066dd1e59c24d88e6921a515adcc4da7335d7770b28bd3047f49e10ad8247edd0022048f7987d46d9395b968ed990e31b6155067e4170bc27c61bc9a2ea5eeafd210228f5dced8b0630d684d0d7f5e4bcc9353a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f8016220c59550e9c4fb7397de61acccb8fa1fe7832c7fbf7039c9d33085c94cd3b4cf4768e901121408011a100801120c4572724e6f42616c616e6365"
	index := int64(0)
	env, err := util.GenEnv(blockByte, index)
	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, db.OpAdd)
	assert.Nil(t, err)

	assert.Equal(t, 4, len(records))
	assert.Equal(t, "transaction/transaction/0xe343916db0587f4fe147d2c535734e5803bf00c28df982ca1e623c06dc997ffd", records[0].Key())
	assert.Equal(t, `{"height_index":49900001,"height":499,"block_time":1565590375,"block_hash":"0xbf374f3030b7650e84b1eeb767e495f167366a57a223a0de2f748ab2f2234376","success":true,"index":1,"hash":"0xe343916db0587f4fe147d2c535734e5803bf00c28df982ca1e623c06dc997ffd","from":"1Q8hGLfoGe63efeWa8fJ4Pnukhkngt6poK","to":"14KEKbYtKKQm4wMthSK9J4La4nAiidGozt","execer":"token","amount":1000000000,"fee":100000,"action_name":"transfer","group_count":0,"is_withdraw":false,"options":{"symbol":"ABCDE","to":"14KEKbYtKKQm4wMthSK9J4La4nAiidGozt"},"assets":[{"exec":"token","symbol":"ABCDE","amount":1000000000}],"next":"","is_para":false}`, string(records[0].Value()))
}

func convertTokenTransferToExec(t *testing.T, convert db.ExecConvert) {
	blockByte := "0a82031220c59550e9c4fb7397de61acccb8fa1fe7832c7fbf7039c9d33085c94cd3b4cf471a2063ce10426854299cacbab76b50e5b7a0a44c163b5f765cb8c76a2e7aee89b90a2220547ef0cec2bc87f25d45c6d36a946a29f59ebdc204ff3528304e133887451de228ea0130fedbed8b063ae5010a0665766d78676f1237300b1a330a03444f4710904e2205746f6b656e2a22313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e51611a6e0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a4730450221009de986618f8372d92345cedfa6f2f1f2df8f8a873f3c278396de2b19ce8ac06202204555e8125375bcb3ca65f8de1110393f2cfa45f120b80758d3eef0e282f4662828f6dced8b0630fe91ad95eddeb292163a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f8016220a27e541df348c9385be73f2faafce4250ba23d4e28f6e609ac975d2a94b247ea68ea0112b20208021a58080312540a2810e09cdd042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152122810d0cedc042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d61521a58080312540a2810c0af052222313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e5161122810d0fd052222313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e51611a7a080812760a22313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e5161122710c0af052221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d61521a2710d0fd052221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152"
	index := int64(0)
	env, err := util.GenEnv(blockByte, index)
	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, db.OpAdd)
	assert.Nil(t, err)

	assert.Equal(t, 5, len(records))
	assert.Equal(t, "transaction/transaction/0xe4a8a1bf1fd560bd1db281d23e08de0f6a38cda464ed6369c9d5b856cc3d4d73", records[0].Key())
	assert.Equal(t, `{"height_index":50100001,"height":501,"block_time":1565590381,"block_hash":"0xe1af68260d64f2150602831c63e11389e8a01f02278c757386624454bcfdda0a","success":true,"index":1,"hash":"0xe4a8a1bf1fd560bd1db281d23e08de0f6a38cda464ed6369c9d5b856cc3d4d73","from":"1Q8hGLfoGe63efeWa8fJ4Pnukhkngt6poK","to":"12hpJBHybh1mSyCijQ2MQJPk7z7kZ7jnQa","execer":"token","amount":10,"fee":100000,"action_name":"transferToExec","group_count":0,"is_withdraw":false,"options":{"symbol":"ABCDE","to":"12hpJBHybh1mSyCijQ2MQJPk7z7kZ7jnQa","exec_name":"token"},"assets":[{"exec":"token","symbol":"ABCDE","amount":10}],"next":"","is_para":false}`, string(records[0].Value()))
}

func convertTokenWithdraw(t *testing.T, convert db.ExecConvert) {
	blockByte := "0a81031220a27e541df348c9385be73f2faafce4250ba23d4e28f6e609ac975d2a94b247ea1a20f1bd069eaa648060c2375688ffdaf35062abe1519d07fda70fed367aa844c8f6222016697f56518f703fa7f4a9a8527c66246730eb3bb14925ce7078a2691168f13d28eb013080dced8b063ae4010a0665766d78676f1237300612330a03444f4710e8072205746f6b656e2a22313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e51611a6d0801122102a1e4229c2cb9c627c51d7f98db8500fec8aafab00396f4a6a8eb81f389e1064f1a46304402201150c5c44a8ea901b9d62be94bd008fe751688f2f648f374d3394d389b6fa5c30220727f4f66d4e451f5a0e95b5f5073d7d8f401fc303b0181eddffa27c4c57d9e7a28f8dced8b0630fcd2c0c5aef5f0a9143a2231365542505734757836484e6451444a7051416570746d454c7070414d393642316758ffff83f8016220ab19ff8c647b95441e911f8797c7759d30712322a42744e6a556e1db607141e168eb0112b20208021a7a080712760a22313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e5161122710d0fd052221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d61521a2710e8f5052221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d61521a58080312540a2810d0fd052222313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e5161122810e8f5052222313268704a4248796268316d537943696a51324d514a506b377a376b5a376a6e51611a58080312540a2810d0cedc042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152122810b8d6dc042221316a51744d46706d4557324a3466674547336f70456f567a6653794d70424d6152"
	index := int64(0)
	env, err := util.GenEnv(blockByte, index)
	assert.Nil(t, err)

	records, err := convert.ConvertTx(env, db.OpAdd)
	assert.Nil(t, err)

	assert.Equal(t, 5, len(records))
	assert.Equal(t, "transaction/transaction/0x78fdb38119ae5eb2a7d47e2800051697619c27a52bfeea4f93531471d8b4fd95", records[0].Key())
	assert.Equal(t, `{"height_index":50400001,"height":504,"block_time":1565590384,"block_hash":"0x9f11a0081bc5095f9be68d5ddd3d1ed8d5782d01f628ea9a38086401ccd9633b","success":true,"index":1,"hash":"0x78fdb38119ae5eb2a7d47e2800051697619c27a52bfeea4f93531471d8b4fd95","from":"1Q8hGLfoGe63efeWa8fJ4Pnukhkngt6poK","to":"12hpJBHybh1mSyCijQ2MQJPk7z7kZ7jnQa","execer":"token","amount":10,"fee":100000,"action_name":"withdraw","group_count":0,"is_withdraw":true,"options":{"symbol":"ABCDE","to":"12hpJBHybh1mSyCijQ2MQJPk7z7kZ7jnQa","exec_name":"token"},"assets":[{"exec":"token","symbol":"ABCDE","amount":10}],"next":"","is_para":false}`, string(records[0].Value()))
}
