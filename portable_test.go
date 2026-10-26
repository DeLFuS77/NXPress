package main

import (
	"os"
	"path/filepath"
	"testing"
)

// With a folder "settings" next to the program, NXPress keeps everything there; without it, in
// the folder of the settings of the user.
func TestDataFolderPortable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	portable := filepath.Join(filepath.Dir(executable), "settings")
	if _, err := os.Stat(portable); err == nil {
		t.Skip("a settings folder is already next to the test program")
	}

	folder, err := dataFolder()
	if err != nil {
		t.Skip(err)
	}
	if folder == portable || webviewFolder() != "" {
		t.Errorf("without the folder, NXPress is portable: %s", folder)
	}

	if err := os.Mkdir(portable, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(portable)
	if folder, _ := dataFolder(); folder != portable {
		t.Errorf("dataFolder = %s, want %s", folder, portable)
	}
	if got, want := webviewFolder(), filepath.Join(portable, "webview"); got != want {
		t.Errorf("webviewFolder = %s, want %s", got, want)
	}
}
