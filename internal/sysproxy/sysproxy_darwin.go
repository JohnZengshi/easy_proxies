//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

type darwinProxy struct {
	baseProxy
	services  []string
	original  map[string]autoProxyState
	appliedTo []string
}

func newPlatformProxy() Proxy {
	return &darwinProxy{}
}

type autoProxyState struct {
	Enabled bool
	URL     string
}

func (p *darwinProxy) Enable(pacURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled {
		return nil
	}
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return fmt.Errorf("networksetup list services: %w", err)
	}
	var services []string
	for _, line := range strings.Split(string(out), "\n") {
		svc := strings.TrimSpace(line)
		if svc == "" || strings.HasPrefix(svc, "*") || strings.Contains(strings.ToLower(svc), "denotes") || strings.Contains(strings.ToLower(svc), "asterisk") {
			continue
		}
		services = append(services, svc)
	}
	if len(services) == 0 {
		return fmt.Errorf("no macOS network services found")
	}
	original := make(map[string]autoProxyState)
	for _, svc := range services {
		original[svc] = readAutoProxyState(svc)
	}
	for i, svc := range services {
		if err := exec.Command("networksetup", "-setautoproxyurl", svc, pacURL).Run(); err != nil {
			for _, done := range services[:i] {
				restoreAutoProxy(done, original[done])
			}
			return fmt.Errorf("set auto proxy %s: %w", svc, err)
		}
	}
	p.services = services
	p.original = original
	p.appliedTo = append([]string(nil), services...)
	p.enabled = true
	p.pacURL = pacURL
	return nil
}

func (p *darwinProxy) Disable() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return nil
	}
	var firstErr error
	for _, svc := range p.appliedTo {
		state := p.original[svc]
		if err := restoreAutoProxy(svc, state); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	p.appliedTo = nil
	p.original = nil
	p.enabled = false
	return firstErr
}

func readAutoProxyState(service string) autoProxyState {
	out, err := exec.Command("networksetup", "-getautoproxyurl", service).Output()
	if err != nil {
		return autoProxyState{}
	}
	var st autoProxyState
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			st.Enabled = strings.Contains(strings.ToLower(line), "yes")
		}
		if strings.HasPrefix(line, "URL:") {
			st.URL = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
			if st.URL == "(null)" {
				st.URL = ""
			}
		}
	}
	return st
}

func restoreAutoProxy(service string, state autoProxyState) error {
	if state.URL != "" {
		return exec.Command("networksetup", "-setautoproxyurl", service, state.URL).Run()
	}
	if err := exec.Command("networksetup", "-setautoproxyurl", service, "(null)").Run(); err != nil {
		return err
	}
	return exec.Command("networksetup", "-setautoproxystate", service, "off").Run()
}
