//go:build windows

package main

import "testing"

func TestDefaultSystemProxy(t *testing.T) {
	if !defaultSystemProxy() {
		t.Fatal("Windows system proxy should default to enabled")
	}
}
