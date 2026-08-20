package sysproxy

import (
	"net"
	"runtime"
	"strconv"
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

// SystemProxyTarget returns the platform-specific value consumed by Proxy.Enable.
// Windows uses a static local proxy so browser routing does not depend on PAC cache.
func SystemProxyTarget(managementListen, listenerAddress string, listenerPort uint16) string {
	if runtime.GOOS != "windows" {
		return PACURL(managementListen)
	}
	return LocalProxyAddress(listenerAddress, listenerPort)
}

// LocalProxyAddress returns the host:port clients use for the mixed listener.
// Older releases wrote this value into the OS web/socks proxy fields, so
// cleanup must still recognize it.
func LocalProxyAddress(listenerAddress string, listenerPort uint16) string {
	host := listenerAddress
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(int(listenerPort)))
}

// Proxy controls the operating-system-level proxy settings.
type Proxy interface {
	Enable(target string) error
	Disable() error
	// CleanupStale clears proxy settings left behind by a previous process when
	// they still point at this project's PAC endpoint or local listener. It is a
	// no-op when the current process already owns the proxy state.
	CleanupStale(pacURL, proxyAddress string) error
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

func (noopProxy) Enable(string) error               { return nil }
func (noopProxy) Disable() error                    { return nil }
func (noopProxy) CleanupStale(string, string) error { return nil }

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
