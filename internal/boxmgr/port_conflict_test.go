//go:build with_clash_api

package boxmgr

import (
	"context"
	"net"
	"strings"
	"testing"

	"easy_proxies/internal/config"
	"easy_proxies/internal/monitor"
)

func TestStart_FailsOnOccupiedNodePort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open conflict listener: %v", err)
	}
	defer listener.Close()

	occupiedPort := uint16(listener.Addr().(*net.TCPAddr).Port)
	cfg := &config.Config{
		Mode:      "multi-port",
		Listener:  config.ListenerConfig{Address: "127.0.0.1", Port: 2323},
		MultiPort: config.MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes: []config.NodeConfig{{
			Name: "occupied",
			URI:  "socks5://127.0.0.1:1",
			Port: occupiedPort,
		}},
	}
	if err := cfg.NormalizeWithPortMap(nil); err != nil {
		t.Fatalf("normalize config: %v", err)
	}

	m := New(cfg, monitor.Config{ProbeTarget: "http://127.0.0.1:1"})
	defer m.Close()

	err = m.Start(context.Background())
	if err == nil {
		t.Fatalf("start unexpectedly succeeded while port %d was occupied", occupiedPort)
	}
	if !strings.Contains(err.Error(), "address already in use") && !strings.Contains(err.Error(), "bind") {
		t.Fatalf("expected bind conflict error, got: %v", err)
	}
}
