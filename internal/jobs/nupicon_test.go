package jobs

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/testnup"
	"github.com/DeLFuS77/NXPress/internal/titledb"
	"github.com/klauspost/compress/zstd"
)

// pngOf is a PNG of one color.
func pngOf(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, fill)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// iconSize tells the size of a JPEG.
func iconSize(t *testing.T, icon []byte) image.Point {
	t.Helper()
	config, err := jpeg.DecodeConfig(bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	return image.Pt(config.Width, config.Height)
}

// useLookup stands in for titledb for the length of a test.
func useLookup(t *testing.T, lookup IconLookup) {
	t.Helper()
	UseIconLookup(lookup)
	t.Cleanup(func() { UseIconLookup(nil) })
}

// A dump folder without control data gives its NUP the icon of its game in titledb, made 256×256;
// with no connection, the NUP is made all the same, without one, and the note tells why.
func TestPackNupTakesTheIconOfTitledb(t *testing.T) {
	asked := uint64(0)
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) {
		asked = id
		return pngOf(t, 512, 300, color.RGBA{0, 0, 255, 255}), nil
	})
	name, note, err := packNup(t.Context(), writeDump(t, "MR2 ROM"), Options{}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if asked != 0x0400D4201C6EA000 {
		t.Errorf("asked for %016X", asked)
	}
	container, err := nup.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if icon := container.Icon(); icon == nil || iconSize(t, icon) != image.Pt(256, 256) {
		t.Fatal("no icon of 256×256")
	}
	if !strings.Contains(note, "its folder has no icon: it carries the icon of its game in titledb, 256×256") {
		t.Errorf("note %q", note)
	}

	// no connection
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) {
		return nil, errors.New("titledb cannot be read: no connection")
	})
	name, note, err = packNup(t.Context(), writeDump(t, "MR2 ROM"), Options{}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if container, err := nup.Open(name); err != nil || container.Icon() != nil {
		t.Errorf("%v", err)
	}
	if !strings.Contains(note, "it has no icon: titledb cannot be read: no connection; one can be given with --nup-icon") {
		t.Errorf("note %q", note)
	}

	// an image of one's own wins over titledb, and --no-nup-icon asks nothing
	own := filepath.Join(t.TempDir(), "own.png")
	os.WriteFile(own, pngOf(t, 100, 100, color.White), 0o644)
	name, note, err = packNup(t.Context(), writeDump(t, "MR2 ROM"), Options{NupIcon: own}, 1, &recording{})
	if err != nil || !strings.Contains(note, "it carries the image given, 256×256") {
		t.Errorf("%q %v", note, err)
	}
	if container, _ := nup.Open(name); container == nil || container.Icon() == nil {
		t.Error("no icon of the image given")
	}
	asked = 0
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) { asked = id; return nil, nil })
	_, note, err = packNup(t.Context(), writeDump(t, "MR2 ROM"), Options{NoNupIcon: true}, 1, &recording{})
	if err != nil || asked != 0 || strings.Contains(note, "icon") {
		t.Errorf("%q %v %x", note, err, asked)
	}
}

// A dump folder with its control icon keeps it, and titledb is not asked.
func TestPackNupKeepsTheIconOfTheFolder(t *testing.T) {
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) {
		t.Error("titledb asked")
		return nil, ErrNoIcon
	})
	dir := writeDump(t, "MR2 ROM")
	control := filepath.Join(dir, "base", "control")
	os.MkdirAll(control, 0o755)
	os.WriteFile(filepath.Join(control, "icon_AmericanEnglish.dat"), testnup.Jpeg, 0o644)
	name, note, err := packNup(t.Context(), dir, Options{}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if container, _ := nup.Open(name); container == nil || !bytes.Equal(container.Icon(), testnup.Jpeg) || strings.Contains(note, "icon") {
		t.Errorf("%q", note)
	}
}

// --fix-nup-icon gives a NUP without an icon the one of its game (an update or a DLC by the ID
// of its game), and leaves one that has an icon.
func TestFixNupIcon(t *testing.T) {
	packages := []testnup.Package{{TitleId: 0x0400486024DED001, Kind: testnup.Addon, Version: 0,
		Files: []testnup.File{{Path: "romfs/dlc.bin", Data: []byte("dlc")}}}}
	path := filepath.Join(t.TempDir(), "dlc.nup")
	os.WriteFile(path, testnup.Build(packages), 0o644)
	asked := uint64(0)
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) {
		asked = titledb.GameID(id)
		return pngOf(t, 40, 80, color.Black), nil
	})
	note, err := FixNupIcon(t.Context(), path, Options{})
	if err != nil || !strings.Contains(note, "it carries the icon of its game in titledb now, 256×256") {
		t.Fatalf("%q %v", note, err)
	}
	if asked != 0x0400486024DEC000 {
		t.Errorf("asked for %016X", asked)
	}
	if container, err := nup.Open(path); err != nil || container.Icon() == nil {
		t.Fatalf("%v", err)
	}
	if _, err := FixNupIcon(t.Context(), path, Options{}); !IsSkipped(err) {
		t.Errorf("again: %v", err)
	}
	// a game titledb does not know: the NUP stays as it was, and the error tells why
	other := filepath.Join(t.TempDir(), "other.nup")
	os.WriteFile(other, testnup.Build(packages), 0o644)
	before, _ := os.ReadFile(other)
	useLookup(t, func(ctx context.Context, id uint64) ([]byte, error) { return nil, ErrNoIcon })
	if _, err := FixNupIcon(t.Context(), other, Options{}); !errors.Is(err, ErrNoIcon) {
		t.Errorf("%v", err)
	}
	// and without a lookup, it tells the setting
	UseIconLookup(nil)
	if _, err := FixNupIcon(t.Context(), other, Options{}); err == nil || !strings.Contains(err.Error(), "settings of the window") {
		t.Errorf("%v", err)
	}
	if now, _ := os.ReadFile(other); !bytes.Equal(now, before) {
		t.Error("changed")
	}
	// the action of the command line
	own := filepath.Join(t.TempDir(), "own.png")
	os.WriteFile(own, pngOf(t, 64, 64, color.White), 0o644)
	if _, note, err := doJob(t.Context(), "fixicon", other, Options{NupIcon: own}, 1, &recording{}); err != nil || !strings.Contains(note, "the image given") {
		t.Errorf("%q %v", note, err)
	}
}

// OnlineIcons reads titledb once and downloads the icon of each game from it.
func TestOnlineIcons(t *testing.T) {
	icon := pngOf(t, 32, 32, color.White)
	lists := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/titles.json.zst":
			lists++
			var out bytes.Buffer
			writer, _ := zstd.NewWriter(&out)
			writer.Write([]byte(`{"0400486024DEC000": {"id": "0400486024DEC000", "iconUrl": "` + server.URL + `/icon.png"}}`))
			writer.Close()
			w.Write(out.Bytes())
		case "/icon.png":
			w.Write(icon)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	old := titledb.IconAddresses
	titledb.IconAddresses = []string{server.URL + "/titles.json.zst"}
	defer func() { titledb.IconAddresses = old }()
	lookup := OnlineIcons(t.TempDir(), server.Client())
	for _, id := range []uint64{0x0400486024DEC000, 0x0400486024DED001} {
		if data, err := lookup(t.Context(), id); err != nil || !bytes.Equal(data, icon) {
			t.Errorf("%016X: %v", id, err)
		}
	}
	if _, err := lookup(t.Context(), 0x0100000000010000); !errors.Is(err, ErrNoIcon) {
		t.Errorf("%v", err)
	}
	if lists != 1 {
		t.Errorf("titledb read %d times", lists)
	}
	// with no connection and no list kept, why
	server.Close()
	if _, err := OnlineIcons(t.TempDir(), http.DefaultClient)(t.Context(), 0x0400486024DEC000); err == nil || !strings.Contains(err.Error(), "titledb cannot be read") {
		t.Errorf("%v", err)
	}
}
