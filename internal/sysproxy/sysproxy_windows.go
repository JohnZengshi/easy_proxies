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
	originalAutoConfigURL string
}

func newPlatformProxy() Proxy {
	return &windowsProxy{}
}

func (p *windowsProxy) Enable(pacURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled {
		return nil
	}
	p.originalAutoConfigURL = regQuery("AutoConfigURL")
	if err := regAdd("AutoConfigURL", "REG_SZ", pacURL); err != nil {
		return err
	}
	notifyInternetSettingsChanged()
	p.enabled = true
	p.pacURL = pacURL
	return nil
}

func (p *windowsProxy) Disable() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return nil
	}
	if p.originalAutoConfigURL != "" {
		_ = regAdd("AutoConfigURL", "REG_SZ", p.originalAutoConfigURL)
	} else {
		_ = exec.Command("reg", "delete", internetSettingsKey, "/v", "AutoConfigURL", "/f").Run()
	}
	notifyInternetSettingsChanged()
	p.enabled = false
	return nil
}

func regQuery(name string) string {
	out, err := exec.Command("reg", "query", internetSettingsKey, "/v", name).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.LastIndex(line, "REG_"); idx >= 0 {
			parts := strings.SplitN(strings.TrimSpace(line[idx+len("REG_"):]), " ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func regAdd(name, kind, value string) error {
	out, err := exec.Command("reg", "add", internetSettingsKey, "/v", name, "/t", kind, "/d", value, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add %s: %s: %w", name, strings.TrimSpace(string(out)), err)
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
