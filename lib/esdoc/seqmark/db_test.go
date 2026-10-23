package seqmark

import (
	"encoding/json"
	"testing"

	"github.com/33cn/externaldb/lib/schema"
	"github.com/stretchr/testify/assert"
)

func TestNewMarkerRecord(t *testing.T) {
	m := &Marker{Seq: 123456, Height: 123450, Hash: "0xabc", Type: schema.SeqTypeAdd}
	r := NewMarkerRecord(m)

	// doc id = seq（十进制）；物理索引由 escli 拼前缀
	assert.Equal(t, TableName, r.Index())
	assert.Equal(t, "123456", r.ID())
	assert.Equal(t, schema.OpAdd, r.OpType())

	var got Marker
	assert.NoError(t, json.Unmarshal(r.Value(), &got))
	assert.Equal(t, *m, got)

	// del seq 同样写 marker（游标不回退）
	d := NewMarkerRecord(&Marker{Seq: 123457, Height: 123450, Hash: "0xabc", Type: schema.SeqTypeDel})
	assert.Equal(t, "123457", d.ID())
	assert.Equal(t, schema.OpAdd, d.OpType())
}
