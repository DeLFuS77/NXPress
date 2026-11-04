package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// -i tells the newest update published beside each title, from the copy the window keeps; with
// no copy, nothing more.
func TestInfoTellsTheNewestPublished(t *testing.T) {
	testkit.Isolate(t)
	keys := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(keys, []byte("header_key = "+testkit.HeaderKey+"\nkey_area_key_application_00 = "+testkit.AreaKey+"\nkey_area_key_application_02 = "+testkit.AreaKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, update, _ := testkit.Game(t, t.TempDir())
	info := func() string {
		var stdout, stderr bytes.Buffer
		if code := RunCommand([]string{"--keys", keys, "-i", update}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit %d: %s%s", code, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	if got := info(); strings.Contains(got, "published") {
		t.Fatalf("with no copy kept: %s", got)
	}

	folder, err := portable.DataFolder()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	list := `{"0100000000010000":{"65536":"2026-01-10","131072":"2026-03-04"}}`
	if err := os.WriteFile(filepath.Join(folder, titledb.FileName), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := info(); !strings.Contains(got, "UPD v65536\n    newest update published: v131072 (2026-03-04)\n") {
		t.Fatalf("with the copy kept: %s", got)
	}
}
