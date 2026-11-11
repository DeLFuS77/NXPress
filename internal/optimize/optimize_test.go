package optimize

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The code of NXDock in native is the one tools/nxdocksync copied: every file NXDOCK.sum lists
// has its SHA-256 (as committed, with LF line ends), and native holds no other code than them and
// the bridge of NXPress.
func TestNXDockCopiesUnchanged(t *testing.T) {
	folder := "native"
	sum, err := os.ReadFile(filepath.Join(folder, "NXDOCK.sum"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{"bridge.h": true, "bridge.cpp": true}
	commit := false
	scanner := bufio.NewScanner(bytes.NewReader(sum))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "commit ") {
			commit = len(strings.TrimPrefix(line, "commit ")) == 40
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("a line of NXDOCK.sum not read: %q", line)
		}
		data, err := os.ReadFile(filepath.Join(folder, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		// a checkout on Windows may give CRLF; NXDock commits LF
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != fields[0] {
			t.Errorf("%s is not the copy of %s of NXDock: run tools/nxdocksync instead of editing it", fields[1], fields[2])
		}
		listed[fields[1]] = true
	}
	if !commit {
		t.Error("NXDOCK.sum names no commit of NXDock")
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".c", ".cpp", ".h", ".hpp":
			if !listed[e.Name()] {
				t.Errorf("%s is in native, not copied from NXDock nor the bridge", e.Name())
			}
		}
	}
}

// The header of an NCA is encrypted as Nintendo encrypts it and back.
func TestXtsBothWays(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plain := make([]byte, 0xC00)
	for i := range plain {
		plain[i] = byte(i * 7)
	}
	data := append([]byte(nil), plain...)
	if err := xts(key, data, true); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(data[:0x200], plain[:0x200]) || bytes.Equal(data[0x200:0x400], data[0x400:0x600]) {
		t.Fatal("not encrypted")
	}
	if err := xts(key, data, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, plain) {
		t.Fatal("not the same back")
	}
	if xts(key[:16], data, true) == nil || xts(key, data[:0x100], true) == nil {
		t.Fatal("a wrong key or a part of a sector taken")
	}
}

// The NSP written with its head at the end is an NSP: its files at their place, read by NXPress
// and by the library.
func TestPfsWriterWritesAnNsp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.nsp")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{strings.Repeat("0", 32) + ".nca", "control.nca", strings.Repeat("0", 32) + ".cnmt.nca"}
	files := [][]byte{bytes.Repeat([]byte("program"), 1000), []byte("control"), []byte("meta")}
	p, err := newPfsWriter(f, names)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range files {
		p.next()
		if _, err := p.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.rename(0, strings.Repeat("a", 32)+".nca"); err != nil {
		t.Fatal(err)
	}
	if p.rename(1, "longer-control.nca") == nil {
		t.Fatal("a name of another length taken")
	}
	if err := p.finish(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if p.head%0x20 != 0 {
		t.Fatalf("a head of %d bytes", p.head)
	}
	nsp, err := openNsp(path)
	if err != nil {
		t.Fatal(err)
	}
	defer nsp.Close()
	want := []string{strings.Repeat("a", 32) + ".nca", "control.nca", strings.Repeat("0", 32) + ".cnmt.nca"}
	for i, name := range want {
		if nsp.order[i] != name {
			t.Fatalf("file %d is %q, not %q", i, nsp.order[i], name)
		}
		got := make([]byte, nsp.entries[name].size)
		if _, err := nsp.ReadAt(got, nsp.entries[name].offset); err != nil || !bytes.Equal(got, files[i]) {
			t.Fatalf("%s holds %q", name, got)
		}
	}
	pfs, err := switchfs.ReadPfs0File(path)
	if err != nil || len(pfs.Files) != 3 {
		t.Fatalf("the library reads it as %v, %v", pfs, err)
	}
}

func TestOptionsChecked(t *testing.T) {
	o, err := Options{}.checked()
	if err != nil || o.Level != 9 || o.Threads != 3 || o.Progress == nil {
		t.Fatalf("%+v %v", o, err)
	}
	if o, _ := (Options{Threads: 32}).checked(); o.Threads != 8 {
		t.Fatalf("%d threads: the code of NXDock packs on 8 at most", o.Threads)
	}
	if _, err := (Options{Level: 13}).checked(); err == nil {
		t.Fatal("level 13 taken")
	}
}
