package config

import (
	"net"
	"strings"
	"testing"
)

func TestStableNodeKey_IgnoresNameAndParamOrder(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		same bool
	}{
		{
			name: "rename only (fragment) keeps identity",
			a:    "vless://uuid-a@a.example.com:443?type=ws&security=tls#OldName",
			b:    "vless://uuid-a@a.example.com:443?type=ws&security=tls#New%20Name%20100GB",
			same: true,
		},
		{
			name: "reordered query params keep identity",
			a:    "vless://uuid-b@b.example.com:443?type=ws&security=tls#B",
			b:    "vless://uuid-b@b.example.com:443?security=tls&type=ws#B",
			same: true,
		},
		{
			name: "different host is a different node",
			a:    "vless://uuid-c@c.example.com:443?type=ws#C",
			b:    "vless://uuid-c@d.example.com:443?type=ws#C",
			same: false,
		},
		{
			name: "different credential is a different node",
			a:    "vless://uuid-1@e.example.com:443#E",
			b:    "vless://uuid-2@e.example.com:443#E",
			same: false,
		},
		{
			name: "changed param value is a different node",
			a:    "vless://uuid-f@f.example.com:443?sni=old.example.com#F",
			b:    "vless://uuid-f@f.example.com:443?sni=new.example.com#F",
			same: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka := stableNodeKey(tt.a)
			kb := stableNodeKey(tt.b)
			if (ka == kb) != tt.same {
				t.Fatalf("stableNodeKey equality = %v, want %v\n a=%q -> %q\n b=%q -> %q",
					ka == kb, tt.same, tt.a, ka, tt.b, kb)
			}
		})
	}
}

// TestNormalizeWithPortMap_PreservesAcrossRefresh proves the end-to-end feature:
// after a subscription refresh that renames a node, reorders another node's
// params, drops a node and adds a new one, the unchanged nodes keep their proxy
// ports and no two nodes collide on a port.
func TestNormalizeWithPortMap_PreservesAcrossRefresh(t *testing.T) {
	const (
		uriA = "vless://uuid-a@a.example.com:443?type=ws&security=tls#NodeA"
		uriB = "vless://uuid-b@b.example.com:443?type=ws&security=tls#NodeB"
		uriC = "vless://uuid-c@c.example.com:443?type=ws&security=tls#NodeC"
	)

	// Initial running config.
	cfg := &Config{
		Mode:      "multi-port",
		MultiPort: MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes: []NodeConfig{
			{URI: uriA},
			{URI: uriB},
			{URI: uriC},
		},
	}
	if err := cfg.NormalizeWithPortMap(nil); err != nil {
		t.Fatalf("initial normalize: %v", err)
	}
	portA, portB := cfg.Nodes[0].Port, cfg.Nodes[1].Port
	if portA == 0 || portB == 0 {
		t.Fatalf("expected non-zero ports, got A=%d B=%d", portA, portB)
	}

	portMap := cfg.BuildPortMap()

	// Refreshed subscription: A renamed, B params reordered, C dropped, D added.
	refreshed := &Config{
		Mode:      "multi-port",
		MultiPort: MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes: []NodeConfig{
			{URI: "vless://uuid-a@a.example.com:443?type=ws&security=tls#NodeA-Renamed-50GB"},
			{URI: "vless://uuid-b@b.example.com:443?security=tls&type=ws#NodeB"},
			{URI: "vless://uuid-d@d.example.com:443?type=ws&security=tls#NodeD"},
		},
	}
	if err := refreshed.NormalizeWithPortMap(portMap); err != nil {
		t.Fatalf("refresh normalize: %v", err)
	}

	if got := refreshed.Nodes[0].Port; got != portA {
		t.Errorf("renamed node A: port changed %d -> %d, want preserved", portA, got)
	}
	if got := refreshed.Nodes[1].Port; got != portB {
		t.Errorf("reordered node B: port changed %d -> %d, want preserved", portB, got)
	}

	// New node D must get a real, distinct port.
	seen := map[uint16]bool{}
	for i, n := range refreshed.Nodes {
		if n.Port == 0 {
			t.Errorf("node %d has no port", i)
		}
		if seen[n.Port] {
			t.Fatalf("port collision detected: %d assigned twice", n.Port)
		}
		seen[n.Port] = true
	}
}

func TestNormalizeWithPortMap_RejectsHybridPoolPortConflict(t *testing.T) {
	const uri = "vless://uuid-a@a.example.com:443?type=ws&security=tls#NodeA"
	cfg := &Config{
		Mode:      "hybrid",
		Listener:  ListenerConfig{Address: "127.0.0.1", Port: 2323},
		MultiPort: MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes:     []NodeConfig{{URI: uri}},
	}
	portMap := map[string]uint16{stableNodeKey(uri): 2323}

	err := cfg.NormalizeWithPortMap(portMap)
	if err == nil || !strings.Contains(err.Error(), "conflicts with hybrid listener port") {
		t.Fatalf("expected hybrid pool conflict error, got %v", err)
	}
	if cfg.Nodes[0].Port == 2323 {
		t.Fatalf("node must not keep a port that conflicts with the hybrid listener")
	}
}

func TestNormalize_RejectsHybridPoolPortConflict(t *testing.T) {
	cfg := &Config{
		Mode:      "hybrid",
		Listener:  ListenerConfig{Address: "127.0.0.1", Port: 2323},
		MultiPort: MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes: []NodeConfig{{
			URI:  "vless://uuid-a@a.example.com:443?type=ws&security=tls#NodeA",
			Port: 2323,
		}},
	}

	err := cfg.normalize()
	if err == nil || !strings.Contains(err.Error(), "conflicts with hybrid listener port") {
		t.Fatalf("expected hybrid pool conflict error, got %v", err)
	}
}

func TestNormalizeWithPortMap_PreservesSavedPortWhenExternalPortIsOccupied(t *testing.T) {
	const uri = "vless://uuid-a@a.example.com:443?type=ws&security=tls#NodeA"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open conflict listener: %v", err)
	}
	defer listener.Close()
	port := uint16(listener.Addr().(*net.TCPAddr).Port)

	cfg := &Config{
		Mode:      "multi-port",
		MultiPort: MultiPortConfig{Address: "127.0.0.1", BasePort: 24000},
		Nodes:     []NodeConfig{{URI: uri}},
	}
	portMap := map[string]uint16{stableNodeKey(uri): port}

	if err := cfg.NormalizeWithPortMap(portMap); err != nil {
		t.Fatalf("normalize with saved port: %v", err)
	}
	if cfg.Nodes[0].Port != port {
		t.Fatalf("saved port changed from %d to %d; published URL must stay stable", port, cfg.Nodes[0].Port)
	}
}
