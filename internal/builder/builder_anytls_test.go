package builder

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestBuildNodeOutbound_AnyTLSPreservesTLSQueryOptions(t *testing.T) {
	uri := "anytls://password@node.example.com:443?sni=front.example.com&allowInsecure=1&alpn=h2,http/1.1&fp=chrome#Node"
	outbound, err := buildNodeOutbound("anytls", uri, false)
	if err != nil {
		t.Fatalf("build anytls node: %v", err)
	}
	if outbound.Type != C.TypeAnyTLS {
		t.Fatalf("expected type %q, got %q", C.TypeAnyTLS, outbound.Type)
	}
	opts, ok := outbound.Options.(*option.AnyTLSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.AnyTLSOutboundOptions, got %T", outbound.Options)
	}
	if opts.TLS == nil {
		t.Fatal("expected AnyTLS TLS options")
	}
	if !opts.TLS.Enabled {
		t.Fatal("expected AnyTLS TLS to be enabled")
	}
	if opts.TLS.ServerName != "front.example.com" {
		t.Fatalf("expected SNI front.example.com, got %q", opts.TLS.ServerName)
	}
	if !opts.TLS.Insecure {
		t.Fatal("expected allowInsecure=1 to propagate")
	}
	if len(opts.TLS.ALPN) != 2 || opts.TLS.ALPN[0] != "h2" || opts.TLS.ALPN[1] != "http/1.1" {
		t.Fatalf("unexpected ALPN list: %v", opts.TLS.ALPN)
	}
	if opts.TLS.UTLS == nil || !opts.TLS.UTLS.Enabled || opts.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatalf("unexpected uTLS options: %+v", opts.TLS.UTLS)
	}
}

func TestBuildNodeOutbound_AnyTLSDefaultsToVerifiedServerName(t *testing.T) {
	outbound, err := buildNodeOutbound("anytls", "anytls://password@node.example.com:443#Node", false)
	if err != nil {
		t.Fatalf("build anytls node: %v", err)
	}
	opts, ok := outbound.Options.(*option.AnyTLSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.AnyTLSOutboundOptions, got %T", outbound.Options)
	}
	if opts.TLS == nil || !opts.TLS.Enabled {
		t.Fatal("expected AnyTLS TLS to be enabled by default")
	}
	if opts.TLS.ServerName != "node.example.com" {
		t.Fatalf("expected default SNI node.example.com, got %q", opts.TLS.ServerName)
	}
	if opts.TLS.Insecure {
		t.Fatal("certificate verification must be enabled by default")
	}
	if len(opts.TLS.ALPN) != 0 {
		t.Fatalf("expected no ALPN by default, got %v", opts.TLS.ALPN)
	}
	if opts.TLS.UTLS != nil {
		t.Fatalf("expected no uTLS by default, got %+v", opts.TLS.UTLS)
	}
}

func TestBuildNodeOutbound_AnyTLSExplicitInsecureOverride(t *testing.T) {
	outbound, err := buildNodeOutbound("anytls", "anytls://password@node.example.com:443?insecure=0#Node", true)
	if err != nil {
		t.Fatalf("build anytls node: %v", err)
	}
	opts, ok := outbound.Options.(*option.AnyTLSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.AnyTLSOutboundOptions, got %T", outbound.Options)
	}
	if opts.TLS == nil || opts.TLS.Insecure {
		t.Fatalf("per-node insecure=0 must override global skip, got %+v", opts.TLS)
	}
}
