package nup

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// withEncoders runs a test with the encoder in Go, and with the zstd library in C when NXPress
// is built with it.
func withEncoders(t *testing.T, run func(t *testing.T)) {
	kinds := []bool{false}
	if nativeZstd {
		kinds = append(kinds, true)
	}
	for _, native := range kinds {
		name := "go"
		if native {
			name = "c"
		}
		t.Run(name, func(t *testing.T) {
			saved := useNative
			useNative = native
			defer func() { useNative = saved }()
			run(t)
		})
	}
}

// codecs gives the codec of each file of a NUP by its package and path.
func codecs(t *testing.T, container *Container) map[string]byte {
	t.Helper()
	found := map[string]byte{}
	for i := range container.Packages {
		err := container.Walk(i, func(path string, index int) error {
			found[Label(container.Packages[i])+"/"+path] = container.Files[index].Codec
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

func TestPackFramesTellTheirSize(t *testing.T) {
	src := oldDump(t)
	withEncoders(t, func(t *testing.T) {
		out, _ := pack(t, src, PackOptions{Level: 3, Workers: 3})
		container, err := Open(out)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		frames, streamed := 0, 0
		for _, f := range container.Files {
			if f.Codec != CodecZstd {
				continue
			}
			head := make([]byte, min(int(f.ExtentSize), 32))
			if _, err := file.ReadAt(head, int64(f.ExtentOffset)); err != nil {
				t.Fatal(err)
			}
			var h zstd.Header
			if err := h.Decode(head); err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			if !h.HasFCS || h.FrameContentSize != f.Size {
				t.Errorf("%s: the frame does not tell its size (%v %d of %d)", f.Name, h.HasFCS, h.FrameContentSize, f.Size)
			}
			frames++
			if f.Size > wholeLimit {
				streamed++
			}
		}
		if frames < 3 || streamed == 0 {
			t.Errorf("%d frames, %d of big files", frames, streamed)
		}
		if _, check, err := Verify(context.Background(), out, 2, 2, nil); err != nil || len(check.Damaged) > 0 {
			t.Errorf("verify: %v %+v", err, check.Damaged)
		}
	})
}

func TestPackSameBytesWhateverTheWorkers(t *testing.T) {
	src := oldDump(t)
	withEncoders(t, func(t *testing.T) {
		one, _ := pack(t, src, PackOptions{Level: 3, Workers: 1, Readers: 1})
		many, _ := pack(t, src, PackOptions{Level: 3, Workers: 6, Readers: 3})
		a, err := os.ReadFile(one)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(many)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("1 worker and 6 workers wrote different NUPs (%d and %d bytes)", len(a), len(b))
		}
	})
}

func TestPackStoresCompressedDataAsItIs(t *testing.T) {
	// zstd frames as Nintendo's .zs files start, followed by data zstd would make smaller
	zs := func(size int) []byte {
		return append([]byte{0x28, 0xB5, 0x2F, 0xFD}, bytes.Repeat([]byte("same "), size/5)...)
	}
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{
		"base/exefs/main":           nso(1),
		"base/exefs/main.npdm":      npdm(0x0400486024DEC000),
		"base/romfs/small.bfres.zs": zs(100_000),
		"base/romfs/big.bntx.zs":    zs(10 << 20),
		"base/romfs/movie.webm":     append([]byte{0x1A, 0x45, 0xDF, 0xA3}, bytes.Repeat([]byte("frame "), 20_000)...),
		"base/romfs/plain.bin":      bytes.Repeat([]byte("same "), 20_000),
		"base/romfs/plain.big":      bytes.Repeat([]byte("same "), (10<<20)/5),
	})
	withEncoders(t, func(t *testing.T) {
		out, _ := pack(t, root, PackOptions{Level: 3, Workers: 2})
		container, err := Open(out)
		if err != nil {
			t.Fatal(err)
		}
		found := codecs(t, container)
		label := Label(container.Packages[0])
		for path, want := range map[string]byte{
			"romfs/small.bfres.zs": CodecRaw,
			"romfs/big.bntx.zs":    CodecRaw,
			"romfs/movie.webm":     CodecRaw,
			"romfs/plain.bin":      CodecZstd,
			"romfs/plain.big":      CodecZstd,
		} {
			if got, ok := found[label+"/"+path]; !ok || got != want {
				t.Errorf("%s: codec %d (%v), want %d", path, got, ok, want)
			}
		}
		if _, check, err := Verify(context.Background(), out, 2, 2, nil); err != nil || len(check.Damaged) > 0 {
			t.Errorf("verify: %v %+v", err, check.Damaged)
		}
	})
}

func TestWithSize(t *testing.T) {
	head := []byte{0x28, 0xB5, 0x2F, 0xFD, 0x04, 0x58}
	got, err := withSize(head, 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x28, 0xB5, 0x2F, 0xFD, 0x84, 0x58, 0x40, 0x4B, 0x4C, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("% x, want % x", got, want)
	}
	got, err = withSize([]byte{0x28, 0xB5, 0x2F, 0xFD, 0x00, 0x60}, 5<<30)
	if err != nil || len(got) != 14 || got[4] != 0xC0 || binary.LittleEndian.Uint64(got[6:]) != 5<<30 {
		t.Errorf("% x %v", got, err)
	}
	// a frame that tells its size already, a single segment, a dictionary, or no zstd at all
	for _, bad := range [][]byte{
		{0x28, 0xB5, 0x2F, 0xFD, 0x80, 0x58},
		{0x28, 0xB5, 0x2F, 0xFD, 0x20, 0x58},
		{0x28, 0xB5, 0x2F, 0xFD, 0x01, 0x58},
		{0x28, 0xB5, 0x2F, 0xFE, 0x00, 0x58},
	} {
		if _, err := withSize(bad, 100); err == nil {
			t.Errorf("% x taken", bad)
		}
	}
	// the header comes in pieces as the library writes it
	var to bytes.Buffer
	frame := &sizedFrame{w: &to, size: 5_000_000}
	for _, piece := range [][]byte{head[:2], head[2:5], append(head[5:], 1, 2, 3)} {
		if n, err := frame.Write(piece); err != nil || n != len(piece) {
			t.Fatal(n, err)
		}
	}
	if !bytes.Equal(to.Bytes(), append(want, 1, 2, 3)) {
		t.Errorf("% x", to.Bytes())
	}
}

func TestCompressedAlready(t *testing.T) {
	for _, data := range [][]byte{
		{0x28, 0xB5, 0x2F, 0xFD, 0x00},
		[]byte("OggS\x00\x02"),
		{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0},
		{0xFF, 0xD8, 0xFF, 0xE0},
		[]byte("\x00\x00\x00\x20ftypisom\x00\x00"),
	} {
		if !compressedAlready(data) {
			t.Errorf("% x not taken as compressed", data)
		}
	}
	for _, data := range [][]byte{nil, []byte("BWAV\x01\x00"), []byte("SARC"), []byte("YB\x07\x00"), {0x28, 0xB5}} {
		if compressedAlready(data) {
			t.Errorf("% x taken as compressed", data)
		}
	}
}

func TestPackStopsWhenCancelledAmongManyWorkers(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"base/exefs/main":      nso(1),
		"base/exefs/main.npdm": npdm(0x0400486024DEC000),
	}
	for i := range 300 {
		files[filepath.ToSlash(filepath.Join("base/romfs/many", string(rune('a'+i%26))+string(rune('a'+i/26))+".bin"))] = bytes.Repeat([]byte{byte(i)}, 1000+i)
	}
	writeTree(t, root, files)
	withEncoders(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		out := filepath.Join(t.TempDir(), "game.nup")
		calls := 0
		_, err := Pack(ctx, root, out, PackOptions{Level: 3, Workers: 8}, func(done int64, total int64) {
			calls++
			if calls == 3 {
				cancel()
			}
		})
		cancel()
		if err == nil {
			t.Error("a cancelled packing gave no error")
		}
	})
}
