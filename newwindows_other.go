//go:build !windows

package main

import "context"

// only the browser of Windows opens a window of its own for a file dropped (#120)
func blockNewWindows(context.Context) {}
