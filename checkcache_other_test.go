//go:build !windows

package main

import "testing"

// notPlugged is where a drive not here was mounted: its folder is gone with it
func notPlugged(t *testing.T) string {
	return "/media/nxpress-not-plugged"
}
