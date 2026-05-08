package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteAtomicNewConcurrentNoReplace(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "result.json")
	const writers = 32
	start := make(chan struct{})
	results := make(chan struct {
		index int
		err   error
	}, writers)
	var wait sync.WaitGroup
	for index := range writers {
		wait.Go(func() {
			<-start
			data := fmt.Appendf(nil, "writer-%02d\n", index)
			results <- struct {
				index int
				err   error
			}{index: index, err: writeAtomicNew(target, data, 0o600)}
		})
	}
	close(start)
	wait.Wait()
	close(results)
	winner := -1
	for result := range results {
		if result.err == nil {
			if winner != -1 {
				t.Fatalf("multiple atomic writers succeeded: %d and %d", winner, result.index)
			}
			winner = result.index
		}
	}
	if winner == -1 {
		t.Fatal("no atomic writer succeeded")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read atomic target: %v", err)
	}
	want := fmt.Appendf(nil, "writer-%02d\n", winner)
	if !bytes.Equal(data, want) {
		t.Fatalf("atomic target = %q, want winning bytes %q", data, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read atomic directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "result.json" {
		t.Fatalf("atomic directory contains leftovers: %v", entries)
	}
}

func TestWriteAtomicNewPreservesExistingBytes(t *testing.T) {
	target := filepath.Join(t.TempDir(), "result.json")
	want := []byte("existing\n")
	if err := os.WriteFile(target, want, 0o640); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	if err := writeAtomicNew(target, []byte("replacement\n"), 0o600); err == nil {
		t.Fatal("atomic writer replaced an existing target")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("existing target = %q, want %q", data, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat existing target: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("existing target mode = %04o, want 0640", info.Mode().Perm())
	}
}
