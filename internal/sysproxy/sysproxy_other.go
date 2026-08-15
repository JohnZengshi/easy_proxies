//go:build !darwin && !windows

package sysproxy

func newPlatformProxy() Proxy {
	return &noopProxy{}
}
