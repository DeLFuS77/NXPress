//go:build windows

package system

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// testExplorer writes the places under a key of the test, deleted at its end: the files of the
// user and the Explorer are not touched.
func testExplorer(t *testing.T) string {
	t.Helper()
	software := fmt.Sprintf(`Software\NXPressTest-%d-%s`, os.Getpid(), t.Name())
	before, notify := explorer, notifyShell
	explorer = explorerKeys{software: software, merged: registry.CURRENT_USER, mergedPath: software + `\Classes`, fileExts: software + `\FileExts`}
	notifyShell = func() {}
	t.Cleanup(func() {
		explorer, notifyShell = before, notify
		if err := deleteTree(software); err != nil {
			t.Errorf("the key of the test stays: %v", err)
		}
	})
	return software
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	key.Close()
	return true
}

// Added, NXPress opens its files with --open, shows in Open with and in Default apps, and leaves
// the types another program opens to it; removed, all of it goes and the rest stays.
func TestFilesAddedAndRemoved(t *testing.T) {
	software := testExplorer(t)
	classes := software + `\Classes`
	executable := `C:\Tools\NXPress\nxpress.exe`
	// a type another program opened before, and one chosen with Open with › Always
	if err := writeString(classes+`\.xci`, "", "Other.File"); err != nil {
		t.Fatal(err)
	}
	if err := writeString(software+`\FileExts\.nsz\UserChoiceLatest\ProgId`, "ProgId", "Another.File"); err != nil {
		t.Fatal(err)
	}
	if place, _ := FilesPlace(executable); place != PlaceMissing {
		t.Fatalf("before: %q", place)
	}
	if err := AddFiles(executable); err != nil {
		t.Fatal(err)
	}
	place, others := FilesPlace(executable)
	if place != PlaceHere || !slices.Equal(others, []string{".nsz", ".xci"}) {
		t.Fatalf("added: %q, others %v", place, others)
	}
	if place, _ := FilesPlace(`D:\Elsewhere\nxpress.exe`); place != PlaceMoved {
		t.Fatalf("for another copy: %q", place)
	}
	if got := readString(registry.CURRENT_USER, classes+`\`+FileClass+`\shell\open\command`, ""); got != `"C:\Tools\NXPress\nxpress.exe" --open "%1"` {
		t.Errorf("command: %s", got)
	}
	if got := readString(registry.CURRENT_USER, classes+`\.nsp`, ""); got != FileClass {
		t.Errorf(".nsp opens with %q", got)
	}
	if got := readString(registry.CURRENT_USER, classes+`\.xci`, ""); got != "Other.File" {
		t.Errorf(".xci opens with %q, not the program it had", got)
	}
	if !exists(t, classes+`\.nup\OpenWithProgids`) || readString(registry.CURRENT_USER, software+`\RegisteredApplications`, "NXPress") == "" {
		t.Error("not in Open with or in Default apps")
	}

	if err := RemoveFiles(); err != nil {
		t.Fatal(err)
	}
	if place, _ := FilesPlace(executable); place != PlaceMissing {
		t.Fatalf("removed: %q", place)
	}
	for _, path := range []string{classes + `\` + FileClass, classes + `\.nsp`, classes + `\.xci\OpenWithProgids`, software + `\NXPress`, software + `\RegisteredApplications`} {
		if exists(t, path) {
			t.Errorf("%s stays", path)
		}
	}
	if got := readString(registry.CURRENT_USER, classes+`\.xci`, ""); got != "Other.File" {
		t.Errorf("the program of .xci went: %q", got)
	}
}

// The menu holds the sections for each type and for folders, in the language given, and each
// entry starts NXPress with --section; it is written again in another language only for this
// copy, and removed with the keys it left empty.
func TestMenuAddedAndRemoved(t *testing.T) {
	software := testExplorer(t)
	classes := software + `\Classes`
	executable := `C:\Tools\NXPress\nxpress.exe`
	texts := ExplorerMenu{Verify: "Verify", Info: "Details", Compress: "Compress", Decompress: "Decompress", Pack: "Combine"}
	if err := AddMenu(executable, texts); err != nil {
		t.Fatal(err)
	}
	if place := MenuPlace(executable); place != PlaceHere {
		t.Fatalf("added: %q", place)
	}
	if place := MenuPlace(`D:\Elsewhere\nxpress.exe`); place != PlaceMoved {
		t.Fatalf("for another copy: %q", place)
	}
	for path, want := range map[string]string{
		`\SystemFileAssociations\.nsp\shell\NXPress\shell\3compress`:   "Compress",
		`\SystemFileAssociations\.nsz\shell\NXPress\shell\3decompress`: "Decompress",
		`\SystemFileAssociations\.nup\shell\NXPress\shell\2info`:       "Details",
		`\Directory\shell\NXPress\shell\1compress`:                     "Compress",
		`\Directory\shell\NXPress\shell\3pack`:                         "Combine",
	} {
		if got := readString(registry.CURRENT_USER, classes+path, "MUIVerb"); got != want {
			t.Errorf("%s: %q", path, got)
		}
	}
	if got := readString(registry.CURRENT_USER, classes+`\SystemFileAssociations\.xci\shell\NXPress\shell\1verify\command`, ""); got != `"C:\Tools\NXPress\nxpress.exe" --section verify "%1"` {
		t.Errorf("command: %s", got)
	}
	if exists(t, classes+`\Directory\shell\NXPress\shell\4info`) || exists(t, classes+`\SystemFileAssociations\.nsp\shell\NXPress\shell\3decompress`) {
		t.Error("an entry that does not fit")
	}

	other := ExplorerMenu{Verify: "Verificar", Info: "Detalles", Compress: "Comprimir", Decompress: "Descomprimir", Pack: "Juntar"}
	if err := RelabelMenu(`D:\Elsewhere\nxpress.exe`, other); err != nil {
		t.Fatal(err)
	}
	if got := readString(registry.CURRENT_USER, classes+`\SystemFileAssociations\.nsp\shell\NXPress\shell\1verify`, "MUIVerb"); got != "Verify" {
		t.Errorf("written again for another copy: %q", got)
	}
	if err := RelabelMenu(executable, other); err != nil {
		t.Fatal(err)
	}
	if got := readString(registry.CURRENT_USER, classes+`\SystemFileAssociations\.nsp\shell\NXPress\shell\1verify`, "MUIVerb"); got != "Verificar" {
		t.Errorf("not written again: %q", got)
	}

	if err := RemoveMenu(); err != nil {
		t.Fatal(err)
	}
	if place := MenuPlace(executable); place != PlaceMissing {
		t.Fatalf("removed: %q", place)
	}
	for _, path := range []string{classes + `\SystemFileAssociations`, classes + `\Directory`} {
		if exists(t, path) {
			t.Errorf("%s stays", path)
		}
	}
}

// The program of a command, quoted or not.
func TestCommandProgram(t *testing.T) {
	for command, want := range map[string]string{
		`"C:\Program Files\NXPress\nxpress.exe" --open "%1"`: `C:\Program Files\NXPress\nxpress.exe`,
		`C:\Tools\nxpress.exe "%1"`:                          `C:\Tools\nxpress.exe`,
		`"C:\unclosed`:                                       "",
		"":                                                   "",
	} {
		if got := commandProgram(command); got != want {
			t.Errorf("%q: %q", command, got)
		}
	}
}
