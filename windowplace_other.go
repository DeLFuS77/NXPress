//go:build !windows

package main

// ownPlacement and restorePlacement are of Windows, which keeps the place of a window itself:
// elsewhere the place goes by the window toolkit.
func ownPlacement() (windowRect, bool, bool) { return windowRect{}, false, false }

func restorePlacement(rect windowRect) bool { return false }
