package main

import (
	"testing"

	"easy_proxies/internal/config"
)

func TestResolveSystemProxyEnabled(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name         string
		flagExplicit bool
		flagValue    bool
		cfgValue     *bool
		want         bool
	}{
		{
			name:         "explicit CLI wins over config",
			flagExplicit: true,
			flagValue:    false,
			cfgValue:     &enabled,
			want:         false,
		},
		{
			name:         "explicit CLI wins over config false",
			flagExplicit: true,
			flagValue:    true,
			cfgValue:     &disabled,
			want:         true,
		},
		{
			name:         "config true wins over CLI default",
			flagExplicit: false,
			flagValue:    false,
			cfgValue:     &enabled,
			want:         true,
		},
		{
			name:         "config false wins over CLI default",
			flagExplicit: false,
			flagValue:    true,
			cfgValue:     &disabled,
			want:         false,
		},
		{
			name:         "no config falls through to CLI default",
			flagExplicit: false,
			flagValue:    false,
			cfgValue:     nil,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgmt := config.ManagementConfig{SystemProxyEnabled: tt.cfgValue}
			if got := resolveSystemProxyEnabled(tt.flagExplicit, tt.flagValue, mgmt); got != tt.want {
				t.Fatalf("resolveSystemProxyEnabled(%v, %v, %+v) = %v, want %v",
					tt.flagExplicit, tt.flagValue, tt.cfgValue, got, tt.want)
			}
		})
	}
}
