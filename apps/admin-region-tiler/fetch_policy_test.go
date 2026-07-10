package main

import "testing"

func TestDefaultTianDiTuPolicyUsesServerSideHeaders(t *testing.T) {
	policy := defaultFetchPolicy("https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", "天地图 vec 电子图")

	if policy.Referer != "" {
		t.Fatalf("expected TianDiTu default policy to omit Referer for server-side keys, got %q", policy.Referer)
	}
	if policy.UserAgent != "" {
		t.Fatalf("expected TianDiTu default policy to omit browser User-Agent for server-side keys, got %q", policy.UserAgent)
	}
	if !policy.RotateHosts {
		t.Fatal("expected TianDiTu host rotation to stay enabled")
	}
	if policy.WorkerCount != 1 {
		t.Fatalf("expected TianDiTu worker count to stay conservative, got %d", policy.WorkerCount)
	}
}
