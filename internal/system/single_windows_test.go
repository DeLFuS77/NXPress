//go:build windows

package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The names of the lock are the ones Wails gives them: a Wails that changed them would leave the
// files of the Explorer opening a window each, unseen by WaitForFirstWindow.
func TestSingleNamesOfWails(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v2").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skip("the source of Wails is not here")
	}
	source, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "internal", "frontend", "desktop", "windows", "single_instance.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`id := "wails-app-" + uniqueId`, `className := id + "-sic"`, `windowName := id + "-siw"`, `mutexName := id + "sim"`} {
		if !strings.Contains(string(source), line) {
			t.Errorf("Wails names its lock otherwise: no %s", line)
		}
	}
	if mutex, class, window := singleNames("x"); mutex != "wails-app-xsim" || class != "wails-app-x-sic" || window != "wails-app-x-siw" {
		t.Errorf("%s %s %s", mutex, class, window)
	}
}

// With no NXPress open there is nothing to wait for.
func TestNoWaitWithoutAWindow(t *testing.T) {
	started := time.Now()
	if WaitForFirstWindow("nxpress-test-none") {
		t.Error("found one")
	}
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("waited %v", waited)
	}
}
