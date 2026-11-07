//go:build !windows

package system

// Only Windows keeps NXPress to one window: there the Explorer starts it with files (#87, #88).

func WaitForFirstWindow(id string) bool { return false }

func StartWindow(id string) (done func(), another bool) { return func() {}, false }

func AllowForeground() {}

func BringToFront() {}
