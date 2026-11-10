package nup

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/bmp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// sided is an image wide or tall, red at its ends and of a color in the middle square.
func sided(width, height int, middle color.Color) image.Image {
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	side := min(width, height)
	for y := range height {
		for x := range width {
			inside := x >= (width-side)/2 && x < (width-side)/2+side && y >= (height-side)/2 && y < (height-side)/2+side
			if inside {
				picture.Set(x, y, middle)
			} else {
				picture.Set(x, y, color.RGBA{255, 0, 0, 255})
			}
		}
	}
	return picture
}

func encodePng(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func encodeJpeg(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// checkIcon tells whether an icon is a JPEG of 256×256 of one color, the middle of the image.
func checkIcon(t *testing.T, icon []byte, want color.RGBA) {
	t.Helper()
	if !bytes.HasPrefix(icon, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("not a JPEG: % x", icon[:min(8, len(icon))])
	}
	picture, err := jpeg.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatal(err)
	}
	if picture.Bounds().Dx() != IconSize || picture.Bounds().Dy() != IconSize {
		t.Fatalf("%v", picture.Bounds())
	}
	near := func(a uint32, b uint8) bool { return int(a>>8)-int(b) < 40 && int(b)-int(a>>8) < 40 }
	for _, point := range []image.Point{{0, 0}, {255, 0}, {0, 255}, {255, 255}, {128, 128}, {2, 128}, {253, 128}} {
		r, g, b, _ := picture.At(point.X, point.Y).RGBA()
		if !near(r, want.R) || !near(g, want.G) || !near(b, want.B) {
			t.Errorf("at %v: %d %d %d, not %v: the image is not cut at its middle", point, r>>8, g>>8, b>>8, want)
		}
	}
}

func TestMakeIconCutsTheMiddleSquare(t *testing.T) {
	green, blue := color.RGBA{0, 200, 0, 255}, color.RGBA{0, 0, 255, 255}
	for name, data := range map[string][]byte{
		"wide png":  encodePng(t, sided(900, 300, green)),
		"tall jpeg": encodeJpeg(t, sided(120, 500, green)),
		"wide bmp":  encodeBmp(t, sided(300, 120, green)),
		"small png": encodePng(t, sided(40, 64, green)),
	} {
		t.Run(name, func(t *testing.T) {
			icon, err := MakeIcon(data)
			if err != nil {
				t.Fatal(err)
			}
			checkIcon(t, icon, green)
		})
	}
	icon, err := MakeIcon(testnup.Webp)
	if err != nil {
		t.Fatal(err)
	}
	checkIcon(t, icon, blue)
	// a transparent PNG is shown on white
	clear := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	if icon, err := MakeIcon(encodePng(t, clear)); err != nil {
		t.Fatal(err)
	} else {
		checkIcon(t, icon, color.RGBA{255, 255, 255, 255})
	}
}

func TestMakeIconRefusesWhatIsNoImage(t *testing.T) {
	for name, data := range map[string][]byte{
		"text":  []byte("not an image"),
		"empty": nil,
		"cut":   encodePng(t, sided(50, 50, color.Black))[:40],
	} {
		if _, err := MakeIcon(data); err == nil {
			t.Errorf("%s: made an icon", name)
		}
	}
	// a PNG that says it is huge is not decoded
	huge := encodePng(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	huge = bytes.Clone(huge)
	copy(huge[16:24], []byte{0, 0, 0x80, 0, 0, 0, 0x80, 0})
	binary.BigEndian.PutUint32(huge[29:], crc32.ChecksumIEEE(huge[12:29]))
	if _, err := MakeIcon(huge); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("huge: %v", err)
	}
}

// noIcon is a game, its update and a DLC without an icon.
func noIcon() []testnup.Package {
	packages := playable()
	packages[0].Meta = []testnup.Record{{Type: 1, Payload: npdm(0x0400486024DEC000)}, {Type: 4, Payload: []byte("1.0.0")},
		{Type: 5, Payload: []byte("Made-up Quest")}, {Type: 6, Payload: []byte("Nobody Games")}, {Type: 8, Payload: []byte("Made-up Dump")}}
	return packages
}

func TestSetIconGivesTheGameAnIcon(t *testing.T) {
	icon, err := MakeIcon(encodePng(t, sided(300, 300, color.RGBA{0, 0, 255, 255})))
	if err != nil {
		t.Fatal(err)
	}
	path := write(t, testnup.Build(noIcon()))
	original, _ := os.ReadFile(path)
	before, err := Open(path)
	if err != nil || before.Icon() != nil {
		t.Fatalf("%v %v", err, before.Icon() != nil)
	}
	set, err := SetIcon(path, icon)
	if err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	after, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Icon(), icon) || !bytes.Equal(after.Packages[0].Icon(), icon) {
		t.Fatal("no icon")
	}
	game := after.Packages[0]
	if game.Name() != "Made-up Quest" || game.Publisher() != "Nobody Games" || game.DisplayVersion() != "1.0.0" ||
		after.Packages[1].DisplayVersion() != before.Packages[1].DisplayVersion() {
		t.Errorf("%+v", after.Packages)
	}
	// what it had stays as it was: the bytes of the files, the metadata as it was
	now, _ := os.ReadFile(path)
	if !bytes.Equal(now[before.Packages[0].metaOffset:][:before.Packages[0].metaSize], original[before.Packages[0].metaOffset:][:before.Packages[0].metaSize]) {
		t.Error("the metadata as it was changed")
	}
	if !bytes.Equal(now[0x38:0x40], original[0x38:0x40]) || !bytes.Equal(now[0x48:0x80], original[0x48:0x80]) {
		t.Error("the header changed beyond the hash and the size")
	}
	if _, check, err := Verify(context.Background(), path, 2, 2, nil); err != nil || len(check.Damaged) != 0 {
		t.Errorf("%v %+v", err, check)
	}
	if got := after.Emulators(); len(got.Refused) != 0 || got.NoStart != "" || got.ZeroBased {
		t.Errorf("emulators %+v", got)
	}
	if _, err := os.Stat(IconBackup(path)); !os.IsNotExist(err) {
		t.Errorf("backup left: %v", err)
	}
	// again: the icon it carries is kept
	other, _ := MakeIcon(encodePng(t, sided(30, 30, color.Black)))
	if set, err := SetIcon(path, other); err != nil || set {
		t.Errorf("again: %v %v", set, err)
	}
	if again, _ := Open(path); !bytes.Equal(again.Icon(), icon) {
		t.Error("the icon was replaced")
	}
}

func TestSetIconKeepsAnIconAndTheCountFromZero(t *testing.T) {
	// a NUP that has one keeps it
	path := write(t, testnup.Build(game()))
	original, _ := os.ReadFile(path)
	if set, err := SetIcon(path, testnup.Jpeg); err != nil || set {
		t.Fatalf("%v %v", set, err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, original) {
		t.Error("a NUP with an icon changed")
	}

	// one counted from 0 gets it counted from 0 too, as the rest of its metadata
	packages := zeroBased()
	packages[0].Meta = append(packages[0].Meta[:1], packages[0].Meta[2:]...)
	path = write(t, testnup.Build(packages))
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	if set, err := SetIcon(path, icon); err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	after, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Packages[0].ZeroBased || after.Packages[0].Name() != "Made-up Quest" || !bytes.Equal(after.Icon(), icon) {
		t.Errorf("%+v", after.Packages[0])
	}
	// and then the fix for the emulators counts it from 1 with the rest
	if fixed, err := FixZeroBased(path); err != nil || fixed == 0 {
		t.Fatalf("%d %v", fixed, err)
	}
	if fixed, _ := Open(path); !bytes.Equal(fixed.Packages[0].Meta[tlvIcon], icon) {
		t.Error("the icon is not where the format puts it")
	}
}

func TestSetIconOnlyDlc(t *testing.T) {
	packages := game()[2:]
	path := write(t, testnup.Build(packages))
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	before, _ := Open(path)
	if before.Icon() != nil {
		t.Skip("the DLC carries an icon")
	}
	if set, err := SetIcon(path, icon); err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	if after, err := Open(path); err != nil || !bytes.Equal(after.Icon(), icon) {
		t.Fatalf("%v", err)
	}
}

func TestSetIconAfterAStop(t *testing.T) {
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	path := write(t, testnup.Build(noIcon()))
	original, _ := os.ReadFile(path)
	if _, err := SetIcon(path, icon); err != nil {
		t.Fatal(err)
	}
	given, _ := os.ReadFile(path)

	// a work that stopped once the icon was written: the copy beside the NUP puts it back as it was
	if err := os.WriteFile(path, given, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IconBackup(path), backupOf(t, original, given), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreIcon(path); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, original) {
		t.Fatal("not put back")
	}
	// a work that stopped before the header was written: the NUP is longer than it says
	if err := os.WriteFile(path, append(bytes.Clone(original), given[len(original):]...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("a NUP longer than its header reads")
	}
	if err := os.WriteFile(IconBackup(path), backupOf(t, original, given), 0o644); err != nil {
		t.Fatal(err)
	}
	if set, err := SetIcon(path, icon); err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, given) {
		t.Error("not given the icon after it was put back")
	}

	// a copy of another NUP is left, and the work stops
	other := write(t, testnup.Build(game()))
	otherData, _ := os.ReadFile(other)
	if err := os.WriteFile(IconBackup(path), backupOf(t, otherData, otherData), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SetIcon(path, icon); err == nil || !strings.Contains(err.Error(), "another NUP") {
		t.Errorf("%v", err)
	}
	if _, err := os.Stat(IconBackup(path)); err != nil {
		t.Error("the copy of another NUP was removed")
	}
}

// backupOf is the copy SetIcon keeps beside a NUP as it was, given the NUP it writes.
func backupOf(t *testing.T, original []byte, given []byte) []byte {
	t.Helper()
	le := func(v uint64) []byte {
		b := make([]byte, 8)
		for i := range b {
			b[i] = byte(v >> (8 * i))
		}
		return b
	}
	tocOffset := uint64(original[0x08]) | uint64(original[0x09])<<8 | uint64(original[0x0A])<<16 | uint64(original[0x0B])<<24
	tocSize := uint64(original[0x10]) | uint64(original[0x11])<<8 | uint64(original[0x12])<<16 | uint64(original[0x13])<<24
	data := []byte(iconBackupMagic)
	data = append(data, le(uint64(len(original)))...)
	data = append(data, le(uint64(len(given)))...)
	data = append(data, given[0x18:0x38]...)
	data = append(data, original[:headerSize]...)
	data = append(data, le(tocOffset)...)
	return append(data, original[tocOffset:tocOffset+tocSize]...)
}

func TestPackGivesTheIconGiven(t *testing.T) {
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	src := oldDump(t)
	out, _ := pack(t, src, PackOptions{Level: 3, Workers: 2, Icon: icon})
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(container.Icon(), icon) || container.Packages[container.IconPackage()].Kind != KindBase {
		t.Error("the icon given is not carried by the game")
	}
	// the icon of the control data wins over the one given
	control := filepath.Join(src, "base", "control")
	writeTree(t, control, map[string][]byte{"icon_AmericanEnglish.dat": testnup.Jpeg})
	if !bytes.Equal(ControlIcon(control), testnup.Jpeg) {
		t.Fatal("ControlIcon")
	}
}

func encodeBmp(t *testing.T, picture image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := bmp.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestReplaceIconChangesTheOneGiven(t *testing.T) {
	path := write(t, testnup.Build(noIcon()))
	first, _ := MakeIcon(encodePng(t, sided(300, 300, color.RGBA{0, 0, 255, 255})))
	if set, err := SetIcon(path, first); err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	// changed once, and again: each time the NUP reads the new one, and the same otherwise (#103)
	for _, shade := range []color.Color{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}} {
		before, _ := Open(path)
		icon, _ := MakeIcon(encodePng(t, sided(256, 256, shade)))
		if set, err := ReplaceIcon(path, icon); err != nil || !set {
			t.Fatalf("%v %v", set, err)
		}
		after, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after.Icon(), icon) {
			t.Fatal("the icon was not replaced")
		}
		if err := sameWithIcon(before, after, 0, icon); err != nil {
			t.Error(err)
		}
		if _, check, err := Verify(context.Background(), path, 2, 2, nil); err != nil || len(check.Damaged) != 0 {
			t.Errorf("%v %+v", err, check)
		}
		if got := after.Emulators(); len(got.Refused) != 0 || got.NoStart != "" {
			t.Errorf("emulators %+v", got)
		}
	}
	// a NUP with only the icon of the game, as packed by others, has it replaced too
	path = write(t, testnup.Build(game()))
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	if set, err := ReplaceIcon(path, icon); err != nil || !set {
		t.Fatalf("%v %v", set, err)
	}
	if after, _ := Open(path); !bytes.Equal(after.Icon(), icon) {
		t.Error("the icon of a NUP with one was not replaced")
	}
}

func TestReplaceIconKeepsTheIconsOfTheGame(t *testing.T) {
	packages := game()
	packages[0].Meta = append(packages[0].Meta, testnup.Record{Type: testnup.LocalizedIcons, Payload: testnup.Localized("en-us", string(testnup.Jpeg))})
	path := write(t, testnup.Build(packages))
	original, _ := os.ReadFile(path)
	container, err := Open(path)
	if err != nil || !container.GameIcons() {
		t.Fatalf("%v %v", err, container != nil && container.GameIcons())
	}
	icon, _ := MakeIcon(encodePng(t, sided(64, 64, color.White)))
	if set, err := ReplaceIcon(path, icon); !errors.Is(err, ErrGameIcons) || set {
		t.Fatalf("%v %v", set, err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, original) {
		t.Error("the NUP changed")
	}
}
