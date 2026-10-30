package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A portable NXPress on Windows sends no notices of the system: readying them writes in the
// registry. Nothing of the window toolkit is called (it would end the test without a window).
func TestPortableSendsNoNotices(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows registers the notices")
	}
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
	if err := os.Mkdir(portable, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(portable)

	a := &App{}
	a.Notify("NXPress", "done")
	a.Notify("NXPress", "done again")
	if a.noticesReady {
		t.Fatal("a portable NXPress readied the notices of the system")
	}
}
