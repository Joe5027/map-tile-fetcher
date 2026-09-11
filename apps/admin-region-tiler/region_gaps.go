package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed geojson/region-gaps.json
var regionGapData []byte

var regionGaps = func() map[string]struct {
	ReasonCode string `json:"reasonCode"`
	Reason     string `json:"reason"`
} { var gaps map[string]struct {
	ReasonCode string `json:"reasonCode"`
	Reason     string `json:"reason"`
}; if err := json.Unmarshal(regionGapData, &gaps); err != nil {
	panic("invalid embedded region gap catalog")
}; return gaps }()

func explainMissingRegion(item *RegionCatalogItem) {
	item.ReasonCode = "region_file_missing"
	item.Reason = "当前区域文件缺失或不可读取。"
	if gap, ok := regionGaps[item.ID]; ok {
		item.ReasonCode, item.Reason = gap.ReasonCode, gap.Reason
	}
}
