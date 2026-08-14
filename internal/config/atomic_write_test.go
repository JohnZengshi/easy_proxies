package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteNodesFileConcurrentWritersProduceCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.txt")
	const writers = 8
	const rounds = 8
	errCh := make(chan error, writers*rounds)

	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for round := 0; round < rounds; round++ {
				nodes := make([]NodeConfig, 0, 32)
				for i := 0; i < 32; i++ {
					nodes = append(nodes, NodeConfig{
						URI: fmt.Sprintf(
							"vless://00000000-0000-0000-0000-000000000000@node-%d-%d-%d.example.com:443#w%d",
							id, round, i, id,
						),
					})
				}
				if err := WriteNodesFile(path, nodes); err != nil {
					errCh <- err
					return
				}
			}
		}(writer)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent node write failed: %v", err)
	}

	loaded, err := LoadNodesFromFile(path)
	if err != nil {
		t.Fatalf("load final nodes file: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("final nodes file is empty")
	}
	for _, node := range loaded {
		if !strings.HasPrefix(node.URI, "vless://") || !strings.Contains(node.URI, "#w") {
			t.Fatalf("final file contains partial/invalid node line %q", node.URI)
		}
	}

	tempFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".nodes.txt.tmp-*"))
	if err != nil {
		t.Fatalf("list temp files: %v", err)
	}
	if len(tempFiles) != 0 {
		t.Fatalf("atomic writer left temp files behind: %v", tempFiles)
	}
}
