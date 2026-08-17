//go:build windows

package sysproxy

import "testing"

func TestProxyServerValue(t *testing.T) {
	got := proxyServerValue("127.0.0.1:2323")
	want := "http=127.0.0.1:2323;https=127.0.0.1:2323"
	if got != want {
		t.Fatalf("proxyServerValue() = %q, want %q", got, want)
	}
}
