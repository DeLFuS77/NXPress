package jobs

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// service is an entry of the service access control: its length less one, the top bit for a
// service offered, and the name.
func service(name string, offered bool) []byte {
	control := byte(len(name) - 1)
	if offered {
		control |= 0x80
	}
	return append([]byte{control}, name...)
}

// word is a kernel capability as its four bytes.
func word(value uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, value)
}

// makeNpdm builds a main.npdm as the games carry it: the META header, then the ACID with its
// file system, service and kernel parts, then the ACI0 with its own.
func makeNpdm() []byte {
	sac := append(append(append(service("fsp-srv", false), service("acc:u0", false)...), service("hid", false)...), service("mysrv", true)...)
	kac := []byte{}
	// threads: lowest priority 59, highest 28, cores 0 to 2
	kac = append(kac, word(0x7|59<<4|28<<10|0<<16|2<<24)...)
	// system calls 0x01 to 0x03 and 0x29 (group 1, bit 17)
	kac = append(kac, word(0xF|(1<<1|1<<2|1<<3)<<5|0<<29)...)
	kac = append(kac, word(0xF|(1<<17)<<5|1<<29)...)
	// a memory map of two words, an interrupt pair with one interrupt, program type, version,
	// handle table and debug flags, then an empty word
	kac = append(kac, word(0x3F|0x1000<<7)...)
	kac = append(kac, word(0x3F|0x10<<7)...)
	kac = append(kac, word(0x7FF|42<<12|0x3FF<<22)...)
	kac = append(kac, word(0x1FFF|1<<14)...)
	kac = append(kac, word(0x3FFF|3<<19|0<<15)...)
	kac = append(kac, word(0x7FFF|512<<16)...)
	kac = append(kac, word(0xFFFF|1<<17)...)
	kac = append(kac, word(0xFFFFFFFF)...)

	fah := make([]byte, 0x1C)
	binary.LittleEndian.PutUint64(fah[4:], 1<<21|1<<63)
	aci := make([]byte, 0x40)
	copy(aci, "ACI0")
	binary.LittleEndian.PutUint64(aci[0x10:], 0x0100000000010000)
	part := func(header []byte, at int, data []byte) []byte {
		binary.LittleEndian.PutUint32(header[at:], uint32(len(header)))
		binary.LittleEndian.PutUint32(header[at+4:], uint32(len(data)))
		return append(header, data...)
	}
	// the offsets are told before each part is added: the header is written as it grows
	aci = part(aci, 0x20, fah)
	aci = part(aci, 0x28, sac)
	aci = part(aci, 0x30, kac)

	acid := make([]byte, 0x240)
	copy(acid[0x200:], "ACID")
	binary.LittleEndian.PutUint32(acid[0x20C:], 1)
	binary.LittleEndian.PutUint64(acid[0x210:], 0x0100000000010000)
	binary.LittleEndian.PutUint64(acid[0x218:], 0x0100000000010FFF)

	meta := make([]byte, 0x80)
	copy(meta, "META")
	meta[0xC] = 1 | 3<<1
	meta[0xE] = 44
	meta[0xF] = 0
	binary.LittleEndian.PutUint32(meta[0x1C:], 0x100000)
	copy(meta[0x20:], "Application")
	binary.LittleEndian.PutUint32(meta[0x78:], uint32(len(meta)))
	binary.LittleEndian.PutUint32(meta[0x7C:], uint32(len(acid)))
	out := append(meta, acid...)
	binary.LittleEndian.PutUint32(out[0x70:], uint32(len(out)))
	binary.LittleEndian.PutUint32(out[0x74:], uint32(len(aci)))
	return append(out, aci...)
}

// main.npdm is read whole: who the program is, the services it uses and offers, and what the
// kernel lets it do.
func TestParseNpdm(t *testing.T) {
	npdm, err := ParseNpdm(makeNpdm())
	if err != nil {
		t.Fatal(err)
	}
	if npdm.Name != "Application" || npdm.Bits != "64-bit" || npdm.AddressSpace != 39 || npdm.Priority != 44 || npdm.Stack != 0x100000 {
		t.Errorf("%+v", npdm)
	}
	if npdm.ProgramID != "0100000000010000" || npdm.ProgramMin != "0100000000010000" || npdm.ProgramMax != "0100000000010FFF" || !npdm.Production {
		t.Errorf("%+v", npdm)
	}
	if npdm.FileSystem != 1<<21|1<<63 {
		t.Errorf("file systems %#x", npdm.FileSystem)
	}
	if !reflect.DeepEqual(npdm.Services, []string{"acc:u0", "fsp-srv", "hid"}) || !reflect.DeepEqual(npdm.Hosts, []string{"mysrv"}) {
		t.Errorf("%v %v", npdm.Services, npdm.Hosts)
	}
	kernel := npdm.Kernel
	want := Kernel{LowestPriority: 59, HighestPriority: 28, MinCore: 0, MaxCore: 2, SystemCalls: []int{1, 2, 3, 0x29}, MemoryMaps: 1, Interrupts: []int{42},
		ProgramType: "application", KernelVersion: "3.0", HandleTable: 512, AllowDebug: true}
	if !reflect.DeepEqual(kernel, want) {
		t.Errorf("kernel\n%+v\nwant\n%+v", kernel, want)
	}
	if got := callRanges(kernel.SystemCalls); got != "0x01-0x03, 0x29" {
		t.Errorf("%q", got)
	}
	if got := PermissionsLine(npdm); got != "permissions: 3 services, 4 system calls, cores 0-2, offers 1 of its own, can be debugged" {
		t.Errorf("%q", got)
	}
	report := strings.Join(permissionsReport(npdm), "\n")
	for _, part := range []string{"Services (3):", "acc:u0, fsp-srv, hid", "Offers (1):", "0x01-0x03, 0x29", "production, program IDs 0100000000010000 to 0100000000010FFF", "not checked"} {
		if !strings.Contains(report, part) {
			t.Errorf("the report lacks %q:\n%s", part, report)
		}
	}
}

// A damaged main.npdm is refused, never read past its end.
func TestParseNpdmRefusesDamage(t *testing.T) {
	good := makeNpdm()
	cases := map[string]func([]byte) []byte{
		"short":          func(data []byte) []byte { return data[:0x40] },
		"no META":        func(data []byte) []byte { copy(data, "ATEM"); return data },
		"ACI0 past end":  func(data []byte) []byte { binary.LittleEndian.PutUint32(data[0x70:], 0xFFFFFFF0); return data },
		"ACID too big":   func(data []byte) []byte { binary.LittleEndian.PutUint32(data[0x7C:], 0xFFFFFFFF); return data },
		"no ACID magic":  func(data []byte) []byte { copy(data[0x80+0x200:], "DICA"); return data },
		"cut in the ACI": func(data []byte) []byte { return data[:len(data)-8] },
	}
	for name, damage := range cases {
		if _, err := ParseNpdm(damage(append([]byte(nil), good...))); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	// a service whose length runs past the end stops the list, not the reading
	services, _ := parseServices([]byte{0x07, 'a', 'b'})
	if len(services) != 0 {
		t.Errorf("%v", services)
	}
}

// exefsOf is an ExeFS (a PFS0) with main and main.npdm, padded to whole sectors.
func exefsOf(npdm []byte) []byte {
	names := []string{"main", "main.npdm"}
	payloads := [][]byte{make([]byte, 0x300), npdm}
	var table []byte
	offsets := []uint32{}
	for _, name := range names {
		offsets = append(offsets, uint32(len(table)))
		table = append(append(table, name...), 0)
	}
	out := make([]byte, 0x10+switchfs.PfsfileEntryTableSize*len(names)+len(table))
	copy(out, "PFS0")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(names)))
	binary.LittleEndian.PutUint32(out[8:], uint32(len(table)))
	position := 0
	for i, payload := range payloads {
		entry := out[0x10+switchfs.PfsfileEntryTableSize*i:]
		binary.LittleEndian.PutUint64(entry[0:], uint64(position))
		binary.LittleEndian.PutUint64(entry[8:], uint64(len(payload)))
		binary.LittleEndian.PutUint32(entry[16:], offsets[i])
		position += len(payload)
	}
	copy(out[0x10+switchfs.PfsfileEntryTableSize*len(names):], table)
	for _, payload := range payloads {
		out = append(out, payload...)
	}
	return append(out, make([]byte, (0x200-len(out)%0x200)%0x200)...)
}

// The permissions are read from the ExeFS of the program of the file, and the full report tells
// them with the kind of package.
func TestPermissionsOfAFile(t *testing.T) {
	testkit.Isolate(t)
	program := testkit.ProgramNcaWith(exefsOf(makeNpdm()), nil)
	path := testkit.NspWith(t, filepath.Join(t.TempDir(), "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
	npdm, ok := ReadPermissions(path)
	if !ok || npdm.Name != "Application" || len(npdm.Services) != 3 {
		t.Fatalf("%+v %v", npdm, ok)
	}
	report, err := Report(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"\nPackage:\n   homebrew", "\nPermissions (main.npdm):\n", "Services (3):"} {
		if !strings.Contains(report, part) {
			t.Errorf("the report lacks %q:\n%s", part, report)
		}
	}
	// a game without main.npdm tells no permissions, and its report no section for them
	plain := testkit.TitleNsp(t, filepath.Join(t.TempDir(), "plain.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 2, nil)
	if _, ok := ReadPermissions(plain); ok {
		t.Fatal("permissions of a program without main.npdm")
	}
	if report, _ := Report(plain, nil); strings.Contains(report, "Permissions") {
		t.Errorf("%s", report)
	}
}
