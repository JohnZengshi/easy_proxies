//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type darwinProxy struct {
	baseProxy
	services  []string
	original  map[string]map[string]proxyState
	appliedTo []string
}

func newPlatformProxy() Proxy {
	return &darwinProxy{}
}

type proxyState struct {
	Enabled bool
	Host    string
	Port    string
}

func (p *darwinProxy) Enable(host string, port int) error {
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
	original := make(map[string]map[string]proxyState)
	for _, svc := range services {
		original[svc] = map[string]proxyState{
			"web":    readProxyState(svc, "-getwebproxy"),
			"secure": readProxyState(svc, "-getsecurewebproxy"),
			"socks":  readProxyState(svc, "-getsocksfirewallproxy"),
		}
	}
	portStr := strconv.Itoa(port)
	for _, svc := range services {
		if err := exec.Command("networksetup", "-setwebproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set web proxy %s: %w", svc, err)
		}
		if err := exec.Command("networksetup", "-setsecurewebproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set secure proxy %s: %w", svc, err)
		}
		if err := exec.Command("networksetup", "-setsocksfirewallproxy", svc, host, portStr).Run(); err != nil {
			return fmt.Errorf("set socks proxy %s: %w", svc, err)
		}
	}
	p.services = services
	p.original = original
	p.appliedTo = append([]string(nil), services...)
	p.enabled = true
	p.host = host
	p.port = port
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
		if state == nil {
			svcState := readProxyState(svc, "-getwebproxy")
			state = map[string]proxyState{
				"web":    svcState,
				"secure": readProxyState(svc, "-getsecurewebproxy"),
				"socks":  readProxyState(svc, "-getsocksfirewallproxy"),
			}
		}
		restoreProxy(svc, "web", state["web"], "-setwebproxy", "-setwebproxystate")
		restoreProxy(svc, "secure", state["secure"], "-setsecurewebproxy", "-setsecurewebproxystate")
		restoreProxy(svc, "socks", state["socks"], "-setsocksfirewallproxy", "-setsocksfirewallproxystate")
	}
	p.appliedTo = nil
	p.original = nil
	p.enabled = false
	return firstErr
}

func readProxyState(service, flag string) proxyState {
	out, err := exec.Command("networksetup", flag, service).Output()
	if err != nil {
		return proxyState{}
	}
	var st proxyState
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			st.Enabled = strings.Contains(strings.ToLower(line), "yes")
		}
		if strings.HasPrefix(line, "Server:") {
			st.Host = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		}
		if strings.HasPrefix(line, "Port:") {
			st.Port = strings.TrimSpace(strings.TrimPrefix(line, "Port:"))
		}
	}
	return st
}

func restoreProxy(service, kind string, st proxyState, setServerFlag, setStateFlag string) {
	if st.Host != "" && st.Port != "" {
		if err := exec.Command("networksetup", setServerFlag, service, st.Host, st.Port).Run(); err != nil {
			_ = err
			return
		}
	}
	stateArg := "off"
	if st.Enabled {
		stateArg = "on"
	}
	_ = exec.Command("networksetup", setStateFlag, service, stateArg).Run()
}
