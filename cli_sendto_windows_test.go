package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// The shortcut of "Send to" is made in any folder: one named O’Brien, which PowerShell took for
// the end of a quoted text, one with letters out of the code page of the system, and one named
// to run code that way, which runs nothing. The shortcut is read back, to tell whether it points
// to this program.
func TestSendToShortcutInAnyFolder(t *testing.T) {
	base := t.TempDir()
	// what a name made to run code would leave, in the working folder
	ran := "nxpress-test-ran"
	for _, name := range []string{
		"O’Brien",
		"it's ‘here’ ‚ ‛",
		"日本語",
		"x’); New-Item " + ran + "; (’",
		"x'); New-Item " + ran + "; ('",
	} {
		folder := filepath.Join(base, name)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(folder, "nxpress.exe")
		os.WriteFile(target, []byte("MZ"), 0o755)
		shortcut := filepath.Join(folder, "NXPress.lnk")
		if err := makeShortcut(shortcut, target); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, err := shortcutTarget(shortcut); err != nil || !strings.EqualFold(got, target) {
			t.Fatalf("%s: the shortcut points to %q (%v)", name, got, err)
		}
		if !shortcutTo(shortcut, target) || shortcutTo(shortcut, filepath.Join(base, "other.exe")) {
			t.Fatalf("%s: to this program, and not to another", name)
		}
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("a folder name ran code")
	}
	// a file that is not a shortcut is taken as it is
	other := filepath.Join(base, "other.lnk")
	os.WriteFile(other, []byte("not a shortcut"), 0o644)
	if _, err := shortcutTarget(other); err == nil || !shortcutTo(other, "x") {
		t.Fatal("a file that is not a shortcut")
	}
}

// shortcutFile is a shortcut with only a LinkInfo, its local path in the code page of the system
// or, with wide, in UTF-16 too.
func shortcutFile(path string, wide bool) []byte {
	header := 0x1C
	inCodePage := path
	if wide {
		header, inCodePage = 0x24, "?"
	}
	info := make([]byte, header)
	binary.LittleEndian.PutUint32(info[4:], uint32(header))
	// a volume and a local path
	binary.LittleEndian.PutUint32(info[8:], 1)
	// the base of the path, and its suffix (empty)
	binary.LittleEndian.PutUint32(info[0x10:], uint32(len(info)))
	info = append(append(info, inCodePage...), 0)
	binary.LittleEndian.PutUint32(info[0x18:], uint32(len(info)))
	info = append(info, 0)
	if wide {
		binary.LittleEndian.PutUint32(info[0x1C:], uint32(len(info)))
		for _, unit := range utf16.Encode([]rune(path)) {
			info = binary.LittleEndian.AppendUint16(info, unit)
		}
		info = append(info, 0, 0)
		binary.LittleEndian.PutUint32(info[0x20:], uint32(len(info)))
		info = append(info, 0, 0)
	}
	binary.LittleEndian.PutUint32(info, uint32(len(info)))
	data := make([]byte, 0x4C)
	binary.LittleEndian.PutUint32(data, 0x4C)
	// it has a LinkInfo
	binary.LittleEndian.PutUint32(data[0x14:], 1<<1)
	return append(data, info...)
}

// The local path of a shortcut is read in both forms it is stored in.
func TestShortcutTargetForms(t *testing.T) {
	folder := t.TempDir()
	for path, wide := range map[string]bool{`C:\Games\nxpress.exe`: false, `C:\ゲーム\nxpress.exe`: true} {
		shortcut := filepath.Join(folder, "test.lnk")
		os.WriteFile(shortcut, shortcutFile(path, wide), 0o644)
		if got, err := shortcutTarget(shortcut); err != nil || got != path {
			t.Errorf("%s: %q %v", path, got, err)
		}
	}
}
