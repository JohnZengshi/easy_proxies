package config

import "testing"

func TestBuildAnyTLSURIPreservesClashTLSMetadata(t *testing.T) {
	proxy := clashProxy{
		Name:              "AnyTLS Node",
		Type:              "anytls",
		Server:            "node.example.com",
		Port:              443,
		Password:          "password",
		ServerName:        "front.example.com",
		SkipCertVerify:    true,
		ClientFingerprint: "chrome",
		ALPN:              []string{"h2", "http/1.1"},
	}
	uri := buildAnyTLSURI(proxy)
	expected := "anytls://password@node.example.com:443?allowInsecure=1&alpn=h2%2Chttp%2F1.1&fp=chrome&sni=front.example.com#AnyTLS+Node"
	if uri != expected {
		t.Fatalf("unexpected AnyTLS URI\nwant: %s\n got: %s", expected, uri)
	}
}
