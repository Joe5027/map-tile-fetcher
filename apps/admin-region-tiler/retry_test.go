package main

import "testing"

func TestTileJobsFromFailureRecordsDeduplicatesAndFilters(t *testing.T) {
	records := []FailureRecord{
		{Z: 3, X: 4, Y: 5, URL: "https://example.test/a/3/4/5.png", Retryable: true},
		{Z: 3, X: 4, Y: 5, URL: "https://example.test/a/3/4/5.png", Retryable: true},
		{Z: 3, X: 4, Y: 6, URL: "https://example.test/a/3/4/6.png", Retryable: false},
		{Z: 3, X: 4, Y: 7, URL: "", Retryable: true},
	}

	jobs := tileJobsFromFailureRecords(records)
	if len(jobs) != 1 {
		t.Fatalf("expected one retry job, got %d", len(jobs))
	}
	if jobs[0].Tile.Z != 3 || jobs[0].Tile.X != 4 || jobs[0].Tile.Y != 5 {
		t.Fatalf("unexpected retry tile: %#v", jobs[0].Tile)
	}
	if jobs[0].URL != "https://example.test/a/3/4/5.png" {
		t.Fatalf("unexpected retry URL: %s", jobs[0].URL)
	}
}
