//go:build darwin

package sysproxy

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

var outputNetworksetup = func(args ...string) ([]byte, error) {
	return exec.Command("networksetup", args...).Output()
}

var runNetworksetup = func(args ...string) error {
	return exec.Command("networksetup", args...).Run()
}

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
	services, err := listNetworkServices()
	if err != nil {
		return err
	}
	original := make(map[string]autoProxyState)
	for _, svc := range services {
		original[svc] = readAutoProxyState(svc)
	}
	for i, svc := range services {
		if err := runNetworksetup("-setautoproxyurl", svc, pacURL); err != nil {
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

func (p *darwinProxy) CleanupStale(pacURL, proxyAddress string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled {
		return nil
	}
	services, err := listNetworkServices()
	if err != nil {
		return err
	}
	var firstErr error
	for _, svc := range services {
		if state := readAutoProxyState(svc); state.URL == pacURL {
			if err := disableAutoProxy(svc); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		// Older releases pointed the web/secure/socks proxy fields straight at
		// the local listener; leaving them set breaks all traffic once this
		// process stops, so clear the ones we own.
		for _, field := range hostProxyFields {
			state := readHostProxyState(field.get, svc)
			if !state.Enabled || state.Address != proxyAddress {
				continue
			}
			if err := runNetworksetup(field.setState, svc, "off"); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type hostProxyField struct {
	get      string
	setState string
}

var hostProxyFields = []hostProxyField{
	{get: "-getwebproxy", setState: "-setwebproxystate"},
	{get: "-getsecurewebproxy", setState: "-setsecurewebproxystate"},
	{get: "-getsocksfirewallproxy", setState: "-setsocksfirewallproxystate"},
}

type hostProxyState struct {
	Enabled bool
	Address string
}

// readHostProxyState parses the "Enabled/Server/Port" block emitted by the
// networksetup -get{web,securewebs,socksfirewall}proxy commands.
func readHostProxyState(getFlag, service string) hostProxyState {
	out, err := outputNetworksetup(getFlag, service)
	if err != nil {
		return hostProxyState{}
	}
	var st hostProxyState
	var server, port string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Enabled:"):
			st.Enabled = strings.Contains(strings.ToLower(line), "yes")
		case strings.HasPrefix(line, "Server:"):
			server = strings.TrimSpace(strings.TrimPrefix(line, "Server:"))
		case strings.HasPrefix(line, "Port:"):
			port = strings.TrimSpace(strings.TrimPrefix(line, "Port:"))
		}
	}
	if server != "" && port != "" && port != "0" {
		st.Address = net.JoinHostPort(server, port)
	}
	return st
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

func listNetworkServices() ([]string, error) {
	out, err := outputNetworksetup("-listallnetworkservices")
	if err != nil {
		return nil, fmt.Errorf("networksetup list services: %w", err)
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
		return nil, fmt.Errorf("no macOS network services found")
	}
	return services, nil
}

func readAutoProxyState(service string) autoProxyState {
	out, err := outputNetworksetup("-getautoproxyurl", service)
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
		if err := runNetworksetup("-setautoproxyurl", service, state.URL); err != nil {
			return err
		}
		if state.Enabled {
			return runNetworksetup("-setautoproxystate", service, "on")
		}
		return runNetworksetup("-setautoproxystate", service, "off")
	}
	return disableAutoProxy(service)
}

func disableAutoProxy(service string) error {
	if err := runNetworksetup("-setautoproxyurl", service, "(null)"); err != nil {
		return err
	}
	return runNetworksetup("-setautoproxystate", service, "off")
}
