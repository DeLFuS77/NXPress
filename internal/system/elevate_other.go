//go:build !windows

package system

import "errors"

// ErrDeclined is the answer No to the question of Windows for the administrator.
var ErrDeclined = errors.New("the permission of the administrator was not given")

// RunElevated is for Windows only: elsewhere NXPress does not ask for the administrator.
func RunElevated(program string, args ...string) (int, error) {
	return 0, errors.New("only Windows runs NXPress as the administrator")
}
