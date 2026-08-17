//go:build !windows

package main

func defaultSystemProxy() bool {
	return false
}
