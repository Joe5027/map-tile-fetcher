package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/paulmach/orb/maptile"
)

func TestConcurrentAtomicTileReplacement(t *testing.T) {
	directory := t.TempDir()
	task := &Task{File: directory, TileMap: TileMap{Format: PNG, Schema: "xyz"}}
	const writers = 32
	start := make(chan struct{})
	errors := make(chan error, writers)
	var done sync.WaitGroup
	for i := 0; i < writers; i++ {
		done.Add(1)
		go func(value byte) {
			defer done.Done()
			<-start
			errors <- saveToFiles(Tile{T: maptile.New(0, 0, 0), C: bytes.Repeat([]byte{value}, 65536)}, task)
		}(byte(i))
	}
	close(start)
	done.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	result, err := os.ReadFile(filepath.Join(directory, "0", "0", "0.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 65536 || !bytes.Equal(result, bytes.Repeat(result[:1], 65536)) {
		t.Fatal("published tile contains mixed or partial writes")
	}
	entries, err := os.ReadDir(filepath.Join(directory, "0", "0"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain: %v %v", entries, err)
	}
}
