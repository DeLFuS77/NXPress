package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// portableFolder is the folder "settings" next to the program, when there is one: NXPress then
// keeps everything it remembers there, the window, the settings of the page and the cache of the
// files checked, and nothing in the folders of the user (#5).
func portableFolder() string {
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

// dataFolder is where NXPress keeps what it remembers: the portable folder, or NXPress in the
// folder of the settings of the user.
func dataFolder() (string, error) {
	if folder := portableFolder(); folder != "" {
		return folder, nil
	}
	folder, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(folder, "NXPress"), nil
}

// webviewFolder is where the page keeps its settings: in the portable folder when there is one,
// else where the window toolkit puts it (""). On Windows that is a folder named after the
// program in the folder of the settings of the user: the one of nxpress.exe is kept, whatever
// the program is named, so a program named otherwise (as "nxpress (1).exe", by a browser that
// had one already) does not lose them.
func webviewFolder() string {
	if folder := portableFolder(); folder != "" {
		return filepath.Join(folder, "webview")
	}
	if folder, err := os.UserConfigDir(); err == nil && runtime.GOOS == "windows" {
		return filepath.Join(folder, "nxpress.exe")
	}
	return ""
}
