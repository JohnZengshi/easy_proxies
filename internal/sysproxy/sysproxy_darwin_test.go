//go:build darwin

package sysproxy

import (
	"fmt"
	"reflect"
	"testing"
)

func resetNetworksetupTestHelpers(t *testing.T) {
	t.Helper()
	oldOutput := outputNetworksetup
	oldRun := runNetworksetup
	t.Cleanup(func() {
		outputNetworksetup = oldOutput
		runNetworksetup = oldRun
	})
}

func TestDarwinCleanupStaleProxy(t *testing.T) {
	resetNetworksetupTestHelpers(t)
	var ran [][]string
	outputNetworksetup = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "-listallnetworkservices":
			return []byte("Wi-Fi\n"), nil
		case "-getautoproxyurl":
			return []byte("Enabled: Yes\nURL: http://127.0.0.1:9091/routing.pac\n"), nil
		case "-getwebproxy", "-getsecurewebproxy", "-getsocksfirewallproxy":
			return []byte("Enabled: No\nServer:\nPort: 0\n"), nil
		}
		return nil, fmt.Errorf("unexpected output call: %v", args)
	}
	runNetworksetup = func(args ...string) error {
		ran = append(ran, args)
		return nil
	}

	p := &darwinProxy{}
	if err := p.CleanupStale("http://127.0.0.1:9091/routing.pac", "127.0.0.1:2323"); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}
	want := [][]string{
		{"-setautoproxyurl", "Wi-Fi", "(null)"},
		{"-setautoproxystate", "Wi-Fi", "off"},
	}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("networksetup calls = %v, want %v", ran, want)
	}
}

func TestDarwinCleanupStaleLeavesOtherProxy(t *testing.T) {
	resetNetworksetupTestHelpers(t)
	var ran [][]string
	outputNetworksetup = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "-listallnetworkservices":
			return []byte("Wi-Fi\n"), nil
		case "-getautoproxyurl":
			return []byte("Enabled: Yes\nURL: http://127.0.0.1:11085/pac/proxy.js\n"), nil
		case "-getwebproxy", "-getsecurewebproxy", "-getsocksfirewallproxy":
			return []byte("Enabled: No\nServer:\nPort: 0\n"), nil
		}
		return nil, fmt.Errorf("unexpected output call: %v", args)
	}
	runNetworksetup = func(args ...string) error {
		ran = append(ran, args)
		return nil
	}

	p := &darwinProxy{}
	if err := p.CleanupStale("http://127.0.0.1:9091/routing.pac", "127.0.0.1:2323"); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}
	if len(ran) != 0 {
		t.Fatalf("other proxy changed: %v", ran)
	}
}

func TestDarwinRestoreAutoProxyPreservesDisabledURL(t *testing.T) {
	resetNetworksetupTestHelpers(t)
	var ran [][]string
	runNetworksetup = func(args ...string) error {
		ran = append(ran, args)
		return nil
	}

	err := restoreAutoProxy("Wi-Fi", autoProxyState{URL: "http://127.0.0.1:11085/pac/proxy.js"})
	if err != nil {
		t.Fatalf("restoreAutoProxy() error = %v", err)
	}
	want := [][]string{
		{"-setautoproxyurl", "Wi-Fi", "http://127.0.0.1:11085/pac/proxy.js"},
		{"-setautoproxystate", "Wi-Fi", "off"},
	}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("networksetup calls = %v, want %v", ran, want)
	}
}

func TestDarwinCleanupStaleClearsLegacyWebProxy(t *testing.T) {
	resetNetworksetupTestHelpers(t)
	var ran [][]string
	outputNetworksetup = func(args ...string) ([]byte, error) {
		switch args[0] {
		case "-listallnetworkservices":
			return []byte("Wi-Fi\n"), nil
		case "-getautoproxyurl":
			return []byte("Enabled: No\nURL: (null)\n"), nil
		case "-getwebproxy", "-getsecurewebproxy":
			return []byte("Enabled: Yes\nServer: 127.0.0.1\nPort: 2323\n"), nil
		case "-getsocksfirewallproxy":
			return []byte("Enabled: Yes\nServer: 127.0.0.1\nPort: 1080\n"), nil
		}
		return nil, fmt.Errorf("unexpected output call: %v", args)
	}
	runNetworksetup = func(args ...string) error {
		ran = append(ran, args)
		return nil
	}

	p := &darwinProxy{}
	if err := p.CleanupStale("http://127.0.0.1:9091/routing.pac", "127.0.0.1:2323"); err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}
	want := [][]string{
		{"-setwebproxystate", "Wi-Fi", "off"},
		{"-setsecurewebproxystate", "Wi-Fi", "off"},
	}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("networksetup calls = %v, want %v", ran, want)
	}
}
