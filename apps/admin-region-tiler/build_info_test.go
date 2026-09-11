package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildInfoAndVersionDoNotInitializeData(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "tiler")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	work := filepath.Join(root, "empty")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "--version")
	cmd.Dir = work
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var info BuildInfo
	if err = json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != "unknown" || info.Commit != "unknown" || info.Dirty != nil {
		t.Fatalf("unknown build claimed identity: %+v", info)
	}
	entries, err := os.ReadDir(work)
	if err != nil || len(entries) != 0 {
		t.Fatal("version query wrote data", entries, err)
	}
	cmd = exec.Command(binary, "-h")
	cmd.Dir = work
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	entries, _ = os.ReadDir(work)
	if len(entries) != 0 {
		t.Fatal("help initialized data")
	}
}
