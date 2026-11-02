// Package portable tells where NXPress keeps what it remembers: next to the program when it is
// portable, else in the folders of the user.
package portable

import (
	"os"
	"path/filepath"
	"runtime"
)

// Folder is the folder "settings" next to the program, when there is one: NXPress then
// keeps everything it remembers there, the window, the settings of the page and the cache of the
// files checked, and nothing in the folders of the user (#5).
func Folder() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	folder := filepath.Join(filepath.Dir(executable), "settings")
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return ""
	}
	return folder
}

// DataFolder is where NXPress keeps what it remembers: the portable folder, or NXPress in the
// folder of the settings of the user.
func DataFolder() (string, error) {
	if folder := Folder(); folder != "" {
		return folder, nil
	}
	folder, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(folder, "NXPress"), nil
}

// WebviewFolder is where the page keeps its settings: in the portable folder when there is one,
// else where the window toolkit puts it (""). On Windows that is a folder named after the
// program in the folder of the settings of the user: the one of nxpress.exe is kept, whatever
// the program is named, so a program named otherwise (as "nxpress (1).exe", by a browser that
// had one already) does not lose them.
func WebviewFolder() string {
	if folder := Folder(); folder != "" {
		return filepath.Join(folder, "webview")
	}
	if folder, err := os.UserConfigDir(); err == nil && runtime.GOOS == "windows" {
		return filepath.Join(folder, "nxpress.exe")
	}
	return ""
}
