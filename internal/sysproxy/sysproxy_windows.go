//go:build windows

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

const internetSettingsKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type windowsProxy struct {
	baseProxy
	originalProxyEnable   registryValue
	originalProxyServer   registryValue
	originalAutoConfigURL registryValue
}

type registryValue struct {
	value  string
	exists bool
}

func newPlatformProxy() Proxy {
	return &windowsProxy{}
}

func (p *windowsProxy) Enable(proxyAddress string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled {
		return nil
	}
	p.originalProxyEnable = regRead("ProxyEnable")
	p.originalProxyServer = regRead("ProxyServer")
	p.originalAutoConfigURL = regRead("AutoConfigURL")
	if err := regAdd("ProxyEnable", "REG_DWORD", "1"); err != nil {
		return err
	}
	if err := regAdd("ProxyServer", "REG_SZ", proxyServerValue(proxyAddress)); err != nil {
		_ = p.restoreLocked()
		return err
	}
	if err := regDelete("AutoConfigURL"); err != nil {
		_ = p.restoreLocked()
		return err
	}
	notifyInternetSettingsChanged()
	p.enabled = true
	p.pacURL = proxyAddress
	return nil
}

func (p *windowsProxy) Disable() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return nil
	}
	err := p.restoreLocked()
	notifyInternetSettingsChanged()
	p.enabled = false
	return err
}

func (p *windowsProxy) restoreLocked() error {
	var firstErr error
	for _, item := range []struct {
		name  string
		kind  string
		value registryValue
	}{
		{"ProxyEnable", "REG_DWORD", p.originalProxyEnable},
		{"ProxyServer", "REG_SZ", p.originalProxyServer},
		{"AutoConfigURL", "REG_SZ", p.originalAutoConfigURL},
	} {
		var err error
		if item.value.exists {
			err = regAdd(item.name, item.kind, item.value.value)
		} else {
			err = regDelete(item.name)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func proxyServerValue(proxyAddress string) string {
	return "http=" + proxyAddress + ";https=" + proxyAddress
}

func regRead(name string) registryValue {
	out, err := exec.Command("reg", "query", internetSettingsKey, "/v", name).Output()
	if err != nil {
		return registryValue{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.LastIndex(line, "REG_"); idx >= 0 {
			parts := strings.SplitN(strings.TrimSpace(line[idx+len("REG_"):]), " ", 2)
			if len(parts) == 2 {
				return registryValue{value: strings.TrimSpace(parts[1]), exists: true}
			}
		}
	}
	return registryValue{}
}

func regAdd(name, kind, value string) error {
	out, err := exec.Command("reg", "add", internetSettingsKey, "/v", name, "/t", kind, "/d", value, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func regDelete(name string) error {
	out, err := exec.Command("reg", "delete", internetSettingsKey, "/v", name, "/f").CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "unable to find") {
		return fmt.Errorf("reg delete %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func notifyInternetSettingsChanged() {
	internetSetOption, err := syscall.LoadDLL("wininet.dll")
	if err != nil {
		return
	}
	proc, err := internetSetOption.FindProc("InternetSetOptionW")
	if err != nil {
		return
	}
	// INTERNET_OPTION_SETTINGS_CHANGED = 39, INTERNET_OPTION_REFRESH = 37
	_, _, _ = proc.Call(0, 39, 0, 0)
	_, _, _ = proc.Call(0, 37, 0, 0)
}
