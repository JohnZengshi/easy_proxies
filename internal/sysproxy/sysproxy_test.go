package sysproxy

import (
	"runtime"
	"testing"
)

func TestPACURL(t *testing.T) {
	got := PACURL("0.0.0.0:9091")
	want := "http://127.0.0.1:9091/routing.pac"
	if got != want {
		t.Fatalf("PACURL() = %q, want %q", got, want)
	}
	got = PACURL("127.0.0.1:9191")
	want = "http://127.0.0.1:9191/routing.pac"
	if got != want {
		t.Fatalf("PACURL() = %q, want %q", got, want)
	}
}

func TestSystemProxyTarget(t *testing.T) {
	got := SystemProxyTarget("0.0.0.0:9091", "0.0.0.0", 2323)
	want := "http://127.0.0.1:9091/routing.pac"
	if runtime.GOOS == "windows" {
		want = "127.0.0.1:2323"
	}
	if got != want {
		t.Fatalf("SystemProxyTarget() = %q, want %q", got, want)
	}
}

func TestNoopProxy(t *testing.T) {
	p := noopProxy{}
	if err := p.Enable("http://127.0.0.1:9091/routing.pac"); err != nil {
		t.Fatal(err)
	}
	if err := p.Disable(); err != nil {
		t.Fatal(err)
	}
}
