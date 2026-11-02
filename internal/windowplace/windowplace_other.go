//go:build !windows

package windowplace

// OwnPlacement and RestorePlacement are of Windows, which keeps the place of a window itself:
// elsewhere the place goes by the window toolkit.
func OwnPlacement() (WindowRect, bool, bool) { return WindowRect{}, false, false }

func RestorePlacement(rect WindowRect) bool { return false }
