package sysproxy

import (
	"runtime"
	"sync"
)

// Proxy controls the operating-system-level proxy settings.
type Proxy interface {
	Enable(host string, port int) error
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

func (noopProxy) Enable(string, int) error { return nil }
func (noopProxy) Disable() error           { return nil }

type baseProxy struct {
	mu      sync.Mutex
	enabled bool
	host    string
	port    int
}

func (p *baseProxy) markEnabled(host string, port int) {
	p.mu.Lock()
	p.enabled = true
	p.host = host
	p.port = port
	p.mu.Unlock()
}

func (p *baseProxy) markDisabled() {
	p.mu.Lock()
	p.enabled = false
	p.mu.Unlock()
}
