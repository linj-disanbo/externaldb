// Package main convert_check — 直连（pull + seq）对账/巡检工具。
//
// 检查内容：
//  1. seq_marker 链是否连续（缺号 = 数据空洞）；
//  2. 每个 marker 的 (height,hash) 与 block_info 是否一致（缺失/不一致都报）；
//  3. 打印旧 db05_last_seq（切换后应停止增长，人工比对即可）。
//
// 发现任何问题退出码非 0，便于接监控/cron。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	l "github.com/33cn/chain33/common/log/log15"
	tml "github.com/BurntSushi/toml"

	"github.com/33cn/externaldb/db"
	"github.com/33cn/externaldb/db/blockinfo"
	"github.com/33cn/externaldb/db/seqmark"
	"github.com/33cn/externaldb/escli"
	"github.com/33cn/externaldb/escli/querypara"
	"github.com/33cn/externaldb/lib/proto"
	"github.com/33cn/externaldb/util"
)

var (
	log        = l.New("module", "convert_check")
	configPath = flag.String("f", "externaldb.toml", "config file")
	fromSeq    = flag.Int64("from", 0, "从该 seq 开始检查（含）")
	toSeq      = flag.Int64("to", 0, "检查到该 seq（含）；0 = 不设上界")
	pageSize   = flag.Int("page", 2000, "每页 marker 条数")
	previewN   = flag.Int("preview", 5, "每类问题最多打印几条")
)

func main() {
	flag.Parse()
	cfg := initCfg(*configPath)
	db.SetVersion(cfg.EsVersion)

	client, err := escli.NewESLongConnect(cfg.ConvertEs.Host, cfg.ConvertEs.Prefix, cfg.EsVersion,
		cfg.ConvertEs.User, cfg.ConvertEs.Pwd)
	if err != nil {
		log.Error("ES connect failed", "host", cfg.ConvertEs.Host, "err", err)
		os.Exit(2)
	}

	code := check(client, cfg, *fromSeq, *toSeq, *pageSize, *previewN)
	os.Exit(code)
}

// check 返回退出码：0=正常，1=发现问题，2=执行失败
func check(client escli.ESClient, cfg *proto.ConfigNew, from, to int64, page, previewN int) int {
	if page <= 0 {
		page = 2000
	}
	var (
		scanned int64
		lastSeq int64 = -1
		maxSeq  int64 = -1
		allGaps []seqRange
		allMiss []*seqmark.Marker
		allDiff []*seqmark.Marker
		biFound int
		pageNum = 1
	)
	for {
		ms, err := pageMarkers(client, from, to, page, pageNum)
		if err != nil {
			log.Error("查询 marker 失败", "page", pageNum, "err", err)
			return 2
		}
		if len(ms) == 0 {
			break
		}
		seqs := make([]int64, 0, len(ms))
		for _, m := range ms {
			seqs = append(seqs, m.Seq)
		}
		allGaps = append(allGaps, checkContinuity(seqs, lastSeq)...)
		lastSeq = seqs[len(seqs)-1]
		maxSeq = lastSeq
		scanned += int64(len(ms))

		// 与 block_info 对账
		bi, err := loadBlockInfo(client, ms)
		if err != nil {
			log.Error("查询 block_info 失败", "page", pageNum, "err", err)
			return 2
		}
		biFound += len(bi)
		d := diffBlockInfo(ms, bi)
		allMiss = append(allMiss, d.Missing...)
		allDiff = append(allDiff, d.Mismatch...)

		log.Info("check page", "page", pageNum, "markers", len(ms),
			"seqRange", fmt.Sprintf("%d-%d", seqs[0], seqs[len(seqs)-1]),
			"missing", len(d.Missing), "mismatch", len(d.Mismatch))

		if len(ms) < page {
			break
		}
		pageNum++
	}

	// 旧 convert 的进度（切换后应停止增长）
	oldSeq, err := util.LastSyncSeq(client, cfg.Convert.AppName)
	if err != nil {
		log.Warn("读取旧 db05_last_seq 失败", "appName", cfg.Convert.AppName, "err", err)
	}

	log.Info("summary", "markers", scanned, "maxSeq", maxSeq, "gaps", len(allGaps),
		"blockInfoMissing", len(allMiss), "blockInfoMismatch", len(allDiff), "oldLastSeq", oldSeq)
	for _, g := range preview(allGaps, previewN) {
		log.Error("缺号区间", "from", g.From, "to", g.To, "count", g.To-g.From+1)
	}
	for _, m := range preview(allMiss, previewN) {
		log.Error("block_info 缺失", "seq", m.Seq, "height", m.Height, "hash", m.Hash)
	}
	for _, m := range preview(allDiff, previewN) {
		log.Error("block_info 不一致", "seq", m.Seq, "markerHeight", m.Height, "hash", m.Hash)
	}
	if scanned == 0 {
		log.Warn("没有扫描到任何 marker（索引未建或游标窗口之外）", "from", from, "to", to)
	}
	// saveBlockInfo=false 时不写 block_info：此时不能把每个 marker 都当成"缺失"
	if scanned > 0 && biFound == 0 {
		log.Warn("block_info 为空（[convert] saveBlockInfo=false？），本次跳过 block_info 对账",
			"markers", scanned)
		allMiss = nil
	}
	if len(allGaps) > 0 || len(allMiss) > 0 || len(allDiff) > 0 {
		return 1
	}
	return 0
}

// pageMarkers 按 seq 升序分页读取 marker，并解码
func pageMarkers(client escli.ESClient, from, to int64, size, pageNum int) ([]*seqmark.Marker, error) {
	r := &querypara.QRange{Key: "seq", RStart: from}
	if to > 0 {
		r.REnd = to
	}
	q := &querypara.Query{
		Range: []*querypara.QRange{r},
		Sort:  []*querypara.QSort{{Key: "seq", Ascending: true}},
		Page:  &querypara.QPage{Size: size, Number: pageNum},
	}
	decode := func(x *json.RawMessage) (interface{}, error) {
		var m seqmark.Marker
		if err := json.Unmarshal(*x, &m); err != nil {
			return nil, err
		}
		return &m, nil
	}
	rs, err := client.Search(seqmark.TableName, seqmark.TableName, q, decode)
	if err != nil {
		return nil, err
	}
	out := make([]*seqmark.Marker, 0, len(rs))
	for _, r := range rs {
		if m, ok := r.(*seqmark.Marker); ok && m != nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// loadBlockInfo 按 marker 的 hash 批量取 block_info（doc id = hash）
func loadBlockInfo(client escli.ESClient, ms []*seqmark.Marker) (map[string]*blockinfo.BlockInfo, error) {
	ids := make([]string, 0, len(ms))
	for _, m := range ms {
		ids = append(ids, m.Hash)
	}
	decode := func(x *json.RawMessage) (interface{}, error) {
		var b blockinfo.BlockInfo
		if err := json.Unmarshal(*x, &b); err != nil {
			return nil, err
		}
		return &b, nil
	}
	rs, err := client.MGet(blockinfo.TableName, blockinfo.TableName, ids, decode)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*blockinfo.BlockInfo, len(rs))
	for _, r := range rs {
		if b, ok := r.(*blockinfo.BlockInfo); ok && b != nil {
			out[b.Hash] = b
		}
	}
	return out, nil
}

func initCfg(path string) *proto.ConfigNew {
	var cfg proto.ConfigNew
	if _, err := tml.DecodeFile(path, &cfg); err != nil {
		log.Error("init config failed", "path", path, "err", err)
		os.Exit(2)
	}
	return &cfg
}
