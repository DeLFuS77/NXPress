package jobs

import (
	"bytes"
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// codeModule is a made-up code module, LZ4 and with its hashes, part of its text data LZ4
// cannot make smaller.
func codeModule() []byte {
	text := make([]byte, 6000)
	rand.New(rand.NewSource(5)).Read(text)
	return testnup.Nso(true, append(bytes.Repeat([]byte("text of the code "), 1000), text...),
		bytes.Repeat([]byte("read-only "), 500), bytes.Repeat([]byte{1, 0, 0, 0}, 1000))
}

// cutModule is a module that lost bytes inside its text, as a dump that lost them.
func cutModule(lost int) []byte {
	module := codeModule()
	return append(append([]byte{}, module[:0x300]...), module[0x300+lost:]...)
}

// writeCodeDump writes a dump folder of a game and its update with the main of the update given.
func writeCodeDump(t *testing.T, main []byte) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Made-up Island")
	for path, data := range map[string][]byte{
		"base/exefs/main.npdm":           npdmOf(0x0400A3A01EE4C000),
		"base/exefs/main":                codeModule(),
		"base/exefs/rtld":                codeModule(),
		"base/romfs/Data/game.bin":       []byte(strings.Repeat("game data ", 500)),
		"updates/655360/exefs/main.npdm": npdmOf(0x0400A3A01EE4C000),
		"updates/655360/exefs/main":      main,
		"updates/655360/romfs/patch.bin": []byte("patched"),
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Verify takes dump folders: their code modules are read by their headers, which is all a dump
// has to be checked by.
func TestVerifyDumpFolders(t *testing.T) {
	good := writeCodeDump(t, codeModule())
	if files, _ := Inputs([]string{good}, Takes["verify"]); len(files) != 1 {
		t.Fatalf("Verify takes the dump folder: %v", files)
	}
	_, note, err := doJob(context.Background(), "verify", good, Options{VerifyLevel: "both", NoCache: true}, 1, testkit.Quiet{})
	if err != nil || note != "3 code modules (NSO) checked by their headers; "+dumpNoHashes+"; a dump folder holds no signatures of Nintendo" {
		t.Fatalf("a good dump: %q %v", note, err)
	}

	damaged := writeCodeDump(t, cutModule(980))
	_, _, err = doJob(context.Background(), "verify", damaged, Options{VerifyLevel: "full", NoCache: true}, 1, testkit.Quiet{})
	if err == nil || !isFailure(err) || err.Error() != codeDamagedDump+" (update v655360: exefs/main: it is 980 bytes shorter than its header says)" {
		t.Fatalf("a dump with its code damaged: %v", err)
	}

	// a dump with no code module has nothing to be checked by
	if _, _, err := doJob(context.Background(), "verify", writeDump(t, "No Code"), Options{NoCache: true}, 1, testkit.Quiet{}); !IsSkipped(err) {
		t.Errorf("a dump with no module: %v", err)
	}
}

// A NUP made of a dump that lost bytes of its code has hashes that match: Verify tells the game to
// be dumped again. Packing such a dump makes the NUP as the dump is, and the work fails with
// what to do.
func TestNupOfADumpWithItsCodeDamaged(t *testing.T) {
	good := writeCodeDump(t, codeModule())
	name, note, err := packNup(t.Context(), good, Options{}, 2, &recording{})
	if err != nil || !strings.Contains(note, "3 code modules (NSO) checked by their headers") {
		t.Fatalf("a good dump: %q %v", note, err)
	}
	_, note, err = verifyNup(t.Context(), name, Options{NoCache: true}, testkit.Quiet{})
	if err != nil || !strings.Contains(note, "files checked by their SHA-256; 3 code modules (NSO) checked by their headers") {
		t.Fatalf("its NUP: %q %v", note, err)
	}

	damaged := writeCodeDump(t, cutModule(31))
	name, _, err = packNup(t.Context(), damaged, Options{}, 2, &recording{})
	if err == nil || !isFailure(err) ||
		err.Error() != codeDamagedDump+" (update v655360: exefs/main: it is 31 bytes shorter than its header says); "+nupMadeAnyway {
		t.Fatalf("a dump with its code damaged: %v", err)
	}
	if _, err := nup.Open(name); err != nil {
		t.Fatalf("the NUP is made as the dump is: %v", err)
	}
	_, _, err = verifyNup(t.Context(), name, Options{NoCache: true}, testkit.Quiet{})
	if err == nil || err.Error() != codeDamagedNup+" (update v655360: exefs/main: it is 31 bytes shorter than its header says)" {
		t.Fatalf("its NUP: %v", err)
	}
}

// The code modules damaged are told by their package and path, the first ones in the error.
func TestCodeDamagedTellsTheFirstModules(t *testing.T) {
	damaged := []nup.Damaged{}
	for i := 0; i < damagedInError+2; i++ {
		damaged = append(damaged, nup.Damaged{Package: "game v0", Path: "exefs/subsdk" + string(rune('0'+i)), Reason: "it is 1 bytes shorter than its header says", Code: true})
	}
	message := codeDamaged(codeDamagedNup, damaged).Error()
	if !strings.HasPrefix(message, codeDamagedNup+" (game v0: exefs/subsdk0: it is 1 bytes shorter than its header says; ") ||
		!strings.HasSuffix(message, "; and 2 more)") {
		t.Errorf("%q", message)
	}
	if codeOnly(nil) || !codeOnly(damaged) || codeOnly(append(damaged, nup.Damaged{Path: "romfs/a.bin"})) {
		t.Error("code only")
	}
}
