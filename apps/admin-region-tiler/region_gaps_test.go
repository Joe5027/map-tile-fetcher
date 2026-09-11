package main

import "testing"

func TestMissingRegionReasonsAndAvailability(t *testing.T) {
	response, _, err := cachedRegionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Missing) != len(regionGaps) {
		t.Fatal("unexpected missing region count", len(response.Missing))
	}
	for _, item := range response.Missing {
		if item.ReasonCode == "" || item.Reason == "" {
			t.Fatal("missing explanation", item.ID)
		}
		if _, err := cachedRegionGeoJSONContent(item.ID); err == nil {
			t.Fatal("missing region was loadable", item.ID)
		}
	}
	for _, item := range response.Available {
		if item.ReasonCode != "" {
			t.Fatal("available region incorrectly marked", item.ID)
		}
	}
}
