package sysproxy

import (
	"net"
	"runtime"
	"sync"
)

// PACURL returns a loopback-reachable URL for the PAC endpoint served by the
// management API. Systems fetch PAC files without authentication, so the URL
// always points at a localhost address even when management listens on all
// interfaces.
func PACURL(managementListen string) string {
	if managementListen == "" {
		managementListen = "127.0.0.1:9091"
	}
	host, port, err := net.SplitHostPort(managementListen)
	if err != nil {
		return "http://127.0.0.1:9091/routing.pac"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/routing.pac"
}

// Proxy controls the operating-system-level proxy settings.
type Proxy interface {
	Enable(pacURL string) error
	Disable() error
}

// Supported reports whether the current platform has an implementation.
func Supported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// New returns a platform-specific system proxy controller. On platforms
// without support the returned proxy is a no-op.
func New() Proxy {
	return newPlatformProxy()
}

type noopProxy struct{}

func (noopProxy) Enable(string) error { return nil }
func (noopProxy) Disable() error      { return nil }

type baseProxy struct {
	mu      sync.Mutex
	enabled bool
	pacURL  string
}

func (p *baseProxy) markEnabled(pacURL string) {
	p.mu.Lock()
	p.enabled = true
	p.pacURL = pacURL
	p.mu.Unlock()
}

func (p *baseProxy) markDisabled() {
	p.mu.Lock()
	p.enabled = false
	p.mu.Unlock()
}
