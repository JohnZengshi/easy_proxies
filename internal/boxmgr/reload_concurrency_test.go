//go:build with_clash_api

package boxmgr

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"easy_proxies/internal/config"
	"easy_proxies/internal/monitor"
)

// TestReload_ConcurrentReloads asserts that two simultaneous TriggerReload
// calls both succeed. Before the fix the second caller landed in the
// currentBox=nil "reloading" window and returned "manager not started".
//
// The SUT requires sing-box to actually start, so the test stands up a real
// loopback SOCKS5 listener (eagerly accepting and closing connections) to
// give the embedded outbound a live target to probe. The probe result is
// irrelevant - the goal is to exercise Manager state, not verify probes.
func TestReload_ConcurrentReloads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode (needs real sing-box startup)")
	}

	socksAddr := startLoopbackListener(t)
	defer stopLoopbackListener(socksAddr)

	mgr := newStartedManagerForReloadTest(t, socksAddr)
	defer mgr.Close()

	cfgCopy := copyCfgForTest(mgr)
	if cfgCopy == nil {
		t.Fatalf("copyCfg returned nil")
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = mgr.Reload(cfgCopy)
	}()
	go func() {
		defer wg.Done()
		errs[1] = mgr.Reload(cfgCopy)
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			if strings.Contains(err.Error(), "manager not started") {
				t.Fatalf("call %d returned the race error: %v", i, err)
			}
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	mgr.mu.RLock()
	current := mgr.currentBox != nil
	mgr.mu.RUnlock()
	if !current {
		t.Fatalf("currentBox left nil after concurrent reloads")
	}
}

// TestReload_AutoRecover asserts that when currentBox is left nil but baseCtx
// is intact (e.g. after a failed rollback consumed the old box but the new
// one never came up), the next Reload starts a fresh box from the existing
// config rather than returning "manager not started". Before the fix this
// state required a process restart to recover.
func TestReload_AutoRecover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode (needs real sing-box startup)")
	}

	socksAddr := startLoopbackListener(t)
	defer stopLoopbackListener(socksAddr)

	mgr := newStartedManagerForReloadTest(t, socksAddr)
	defer mgr.Close()

	// Simulate the post-failed-rollback state: the old box has been fully
	// closed (its ports released) but currentBox is still nil because the
	// rollback never produced a replacement. baseCtx stays intact - that is
	// the discriminator between "permanent failure" and "never started".
	mgr.mu.Lock()
	oldBox := mgr.currentBox
	mgr.currentBox = nil
	mgr.mu.Unlock()
	if oldBox != nil {
		if err := oldBox.Close(); err != nil {
			t.Fatalf("close old box: %v", err)
		}
	}

	// Give the OS a moment to release the just-closed listener. Reload itself
	// waits 500ms after closing the old box in the non-nil case, but here we
	// closed manually so the wait is on us.
	time.Sleep(800 * time.Millisecond)

	cfgCopy := copyCfgForTest(mgr)
	if cfgCopy == nil {
		t.Fatalf("copyCfg returned nil")
	}

	if err := mgr.Reload(cfgCopy); err != nil {
		if strings.Contains(err.Error(), "manager not started") {
			t.Fatalf("auto-recovery did not happen: %v", err)
		}
		t.Fatalf("Reload failed: %v", err)
	}

	mgr.mu.RLock()
	current := mgr.currentBox != nil
	mgr.mu.RUnlock()
	if !current {
		t.Fatalf("currentBox still nil after auto-recovery reload")
	}
}

// startLoopbackListener brings up an eager-accepting TCP listener and returns
// the host:port string so a node URI can target it. The outbound probe will
// fail (no SOCKS handshake) but the sing-box process starts cleanly and the
// reload path is exercised end-to-end. The listener is closed by the caller
// via stopLoopbackListener.
func startLoopbackListener(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().String()
}

// stopLoopbackListener closes the eager-accept listener. Kept for symmetry
// with startLoopbackListener; defer-friendly. Real teardown happens via the
// t.Cleanup registered in startLoopbackListener.
func stopLoopbackListener(addr string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
	}
	_ = port
}

// newStartedManagerForReloadTest builds a Manager with a single socks5 node
// pointing at the loopback listener and starts it. The probe target is left
// blank so the embedded health check marks nodes available without actually
// attempting a real probe - we just need Start to return.
func newStartedManagerForReloadTest(t *testing.T, socksAddr string) *Manager {
	t.Helper()

	// Reserve two ephemeral ports: one for the embedded Clash API and one
	// for the per-node listener. Both could collide with a live easy_proxies
	// instance (9092 default clash port, 24000+ default node ports), so we
	// grab them from the kernel instead of relying on hardcoded defaults.
	clashL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve clash port: %v", err)
	}
	clashPort := uint16(clashL.Addr().(*net.TCPAddr).Port)
	_ = clashL.Close()

	nodeL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve node port: %v", err)
	}
	nodePort := uint16(nodeL.Addr().(*net.TCPAddr).Port)
	_ = nodeL.Close()

	t.Cleanup(func() {
		// Best-effort cleanup: dial to nudge the OS to release the bound ports
		// promptly when the test process exits.
		for _, p := range []uint16{clashPort, nodePort} {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(p)), 50*time.Millisecond)
			if err == nil {
				_ = conn.Close()
			}
		}
	})

	cfg := &config.Config{
		Mode:     "multi-port",
		Listener: config.ListenerConfig{Address: "127.0.0.1", Port: 0},
		MultiPort: config.MultiPortConfig{
			Address:  "127.0.0.1",
			BasePort: nodePort,
		},
		Nodes: []config.NodeConfig{{
			Name: "n1",
			URI:  "socks5://" + socksAddr,
			Port: nodePort,
		}},
		Experimental: config.ExperimentalConfig{
			ClashAPIPort: int(clashPort),
		},
	}
	if err := cfg.NormalizeWithPortMap(nil); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	m := New(cfg, monitor.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = fmt.Sprintf // keep fmt imported for future debug prints
	return m
}

// copyCfgForTest returns a shallow copy of m.cfg so Reload-style callers
// have their own pointer (NormalizeWithPortMap mutates).
func copyCfgForTest(m *Manager) *config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg == nil {
		return nil
	}
	cp := *m.cfg
	cp.Nodes = append([]config.NodeConfig(nil), m.cfg.Nodes...)
	return &cp
}
