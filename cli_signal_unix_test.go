//go:build !windows

package main

import (
	"syscall"
	"testing"
	"time"
)

// A SIGTERM ends a watch as Ctrl+C does, and NXPress returns instead of being cut off.
func TestWatchEndsOnSigterm(t *testing.T) {
	folder := t.TempDir()
	ended := make(chan int, 1)
	go func() {
		_, code := runForTest(t, "--fat32", "--watch", folder)
		ended <- code
	}()
	time.Sleep(300 * time.Millisecond)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	select {
	case code := <-ended:
		if code != 0 {
			t.Fatalf("exit code %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the watch goes on after SIGTERM")
	}
}
