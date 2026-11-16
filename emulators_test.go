package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// every emulator of the list has a name and a page on https, none twice, and only web pages are opened
func TestEmulatorList(t *testing.T) {
	names, addresses := map[string]bool{}, map[string]bool{}
	count := 0
	for _, group := range emulatorGroups {
		if group.Title == "" || len(group.Emulators) == 0 {
			t.Errorf("a section with no title or no emulator: %+v", group)
		}
		// a family is told once: its emulators are one after the other
		seen, last := map[string]bool{}, ""
		for _, emulator := range group.Emulators {
			if emulator.Family != last && emulator.Family != "" {
				if seen[emulator.Family] {
					t.Errorf("%s: the family %s is split in two", group.Title, emulator.Family)
				}
				seen[emulator.Family] = true
			}
			last = emulator.Family
			count++
			if emulator.Name == "" || !strings.HasPrefix(emulator.Address, "https://") {
				t.Errorf("not an emulator with a name and a page on https: %+v", emulator)
			}
			if names[emulator.Name] || addresses[emulator.Address] {
				t.Errorf("%s is listed twice", emulator.Name)
			}
			names[emulator.Name], addresses[emulator.Address] = true, true
		}
	}
	if count != 22 {
		t.Errorf("%d emulators are listed, 22 were asked for", count)
	}
	// the pages of the list of the person are web pages of any site; nothing else is opened
	app := &App{}
	for _, address := range []string{"", "https://", "ftp://example.test/x", "file:///c:/windows/system32/calc.exe", "javascript:alert(1)", "github.com/x", " https://github.com/x", "https://github.com/a b", "https://a.test/\\x"} {
		if err := app.OpenEmulatorPage(address); err == nil {
			t.Errorf("%q, not a web page, was opened", address)
		}
	}
	for _, address := range []string{"https://github.com/zortos293/Lichii", "http://example.test/x?y=1#z", "https://git.eden-emu.dev/"} {
		if !emulatorPage(address) {
			t.Errorf("%q is a web page, and was refused", address)
		}
	}
}

// a file of emulators is read as text, without the mark some editors put first, and a big file, a folder or a
// file that is not text is refused
func TestReadEmulatorsFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if text, err := readEmulatorsFile(write("a.json", []byte("\ufeff{\"a\":1}"))); err != nil || text != `{"a":1}` {
		t.Errorf("read %q, %v", text, err)
	}
	if _, err := readEmulatorsFile(write("b.json", []byte{0xff, 0xfe, 0x00, 0x80})); err == nil {
		t.Error("a file that is not text was read")
	}
	if _, err := readEmulatorsFile(write("c.json", make([]byte, maxEmulatorsFile+1))); err == nil {
		t.Error("a big file was read")
	}
	if _, err := readEmulatorsFile(dir); err == nil {
		t.Error("a folder was read")
	}
	if _, err := readEmulatorsFile(filepath.Join(dir, "none.json")); err == nil {
		t.Error("a file that is not there was read")
	}
}
