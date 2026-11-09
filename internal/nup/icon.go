package nup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// A NUP made from a dump folder without control data carries no icon, and emulators show it with
// none. An icon from elsewhere (the one of the game in titledb, or an image of one's own) is
// made the size of the icons of the console, a JPEG of 256×256, and given to the NUP: when it is
// packed (PackOptions.Icon), or afterwards (SetIcon) to one made already without one.

// IconSize is the side of the icons of the console, in pixels.
const IconSize = 256

const (
	// an image of more pixels than this is not an icon: it is not decoded, so a crafted one cannot
	// fill the memory
	maxIconPixels = 8192 * 8192
	// an image file bigger than this is not read
	MaxIconFile = 32 << 20
)

// MakeIcon makes an image (PNG, JPEG, WEBP or BMP) the icon of a game: cut square at its middle, made
// 256×256 and written as a JPEG, as the icons of the console are.
func MakeIcon(data []byte) ([]byte, error) {
	if len(data) > MaxIconFile {
		return nil, errors.New("the image is too big to be an icon")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the image cannot be read (PNG, JPEG, WEBP or BMP): %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxIconPixels {
		return nil, fmt.Errorf("the image is %d×%d pixels, too many for an icon", config.Width, config.Height)
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the image cannot be read (PNG, JPEG, WEBP or BMP): %w", err)
	}
	bounds := picture.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	square := image.Rect(0, 0, side, side).Add(image.Pt(bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2))
	icon := image.NewRGBA(image.Rect(0, 0, IconSize, IconSize))
	// a transparent image is shown on white, as JPEG keeps no transparency
	draw.Draw(icon, icon.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(icon, icon.Bounds(), picture, square, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, icon, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ControlIcon is the icon a control folder gives, nil without one.
func ControlIcon(dir string) []byte {
	return controlRecords(dir)[tlvIcon]
}

// IconPackage is the package of a NUP that carries the icon of its game: the game, else the
// first package, as the format asks of a packer.
func (c *Container) IconPackage() int {
	for i, pkg := range c.Packages {
		if pkg.Kind == KindBase {
			return i
		}
	}
	return 0
}

// IconBackup is the copy of the header and the table of contents of a NUP kept while an icon is
// given to it.
func IconBackup(path string) string {
	return path + ".nxpress-icon"
}

const iconBackupMagic = "NXPICON1"

// SetIcon gives a NUP that carries no icon the one given (made by MakeIcon), and tells whether it
// did: a NUP that carries one keeps it. Nothing of what the NUP has changes: the metadata of the
// package of the game is written again with the icon, after the end of the NUP, and the table of
// contents points to it; the data of the files, and the metadata as it was, stay where they are.
// The header and the table of contents as they were are kept beside the NUP while it is written,
// and put back when it does not read the same afterwards; a NUP left with such a copy by a work
// that stopped gets them back before anything else.
func SetIcon(path string, icon []byte) (bool, error) {
	if len(icon) == 0 || !bytes.HasPrefix(icon, []byte{0xFF, 0xD8, 0xFF}) && !bytes.HasPrefix(icon, []byte("\x89PNG\r\n\x1a\n")) {
		return false, errors.New("the icon is not a JPEG or PNG image")
	}
	if len(icon) > maxMetaSize/32 {
		return false, errors.New("the icon is too big")
	}
	if err := restoreIcon(path); err != nil {
		return false, err
	}
	if err := restoreMeta(path); err != nil {
		return false, err
	}
	before, err := Open(path)
	if err != nil {
		return false, err
	}
	if before.Icon() != nil {
		return false, nil
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer file.Close()
	le := binary.LittleEndian
	header := make([]byte, headerSize)
	if _, err := file.ReadAt(header, 0); err != nil {
		return false, err
	}
	tocOffset, tocSize := le.Uint64(header[0x08:]), le.Uint64(header[0x10:])
	toc := make([]byte, tocSize)
	if _, err := file.ReadAt(toc, int64(tocOffset)); err != nil {
		return false, err
	}

	// the metadata of the package as it is, without a record of an icon that is no image, and the
	// icon after it, with its type counted as the block counts them
	index := before.IconPackage()
	pkg := before.Packages[index]
	old := make([]byte, pkg.metaSize)
	if _, err := file.ReadAt(old, int64(pkg.metaOffset)); err != nil {
		return false, err
	}
	kind := uint16(tlvIcon)
	if pkg.ZeroBased {
		kind--
	}
	var block bytes.Buffer
	for pos := 0; pos+tlvHeadSize <= len(old); {
		size := int(le.Uint32(old[pos+4:]))
		if size < 0 || size > len(old)-pos-tlvHeadSize {
			return false, errors.New("a record of its metadata runs past the end of its block")
		}
		if le.Uint16(old[pos:]) != kind {
			block.Write(old[pos : pos+tlvHeadSize+size])
		}
		pos += tlvHeadSize + size
	}
	var head [tlvHeadSize]byte
	le.PutUint16(head[0:], kind)
	le.PutUint32(head[4:], uint32(len(icon)))
	block.Write(head[:])
	block.Write(icon)
	if block.Len() > maxMetaSize {
		return false, errors.New("its metadata would be too big with the icon")
	}

	oldSize := before.FileSize
	at := (oldSize + 15) / 16 * 16
	end := at + int64(block.Len())
	newToc := bytes.Clone(toc)
	record := newToc[tocHeadSize+index*packageSize:]
	le.PutUint64(record[0x20:], uint64(at))
	le.PutUint32(record[0x28:], uint32(block.Len()))
	newHeader := bytes.Clone(header)
	sum := sha256.Sum256(newToc)
	copy(newHeader[0x18:0x38], sum[:])
	le.PutUint64(newHeader[0x40:], uint64(end))

	// the header and the table of contents as they were, beside the NUP
	var backup bytes.Buffer
	backup.WriteString(iconBackupMagic)
	binary.Write(&backup, le, uint64(oldSize))
	binary.Write(&backup, le, uint64(end))
	backup.Write(newHeader[0x18:0x38])
	backup.Write(header)
	binary.Write(&backup, le, tocOffset)
	backup.Write(toc)
	if err := writeSynced(IconBackup(path), backup.Bytes()); err != nil {
		return false, fmt.Errorf("the copy of its table of contents cannot be written beside it: %w", err)
	}
	undo := func(err error) (bool, error) {
		file.Close()
		return false, errors.Join(err, restoreIcon(path))
	}
	if _, err := file.WriteAt(make([]byte, at-oldSize), oldSize); err != nil {
		return undo(err)
	}
	if _, err := file.WriteAt(block.Bytes(), at); err != nil {
		return undo(err)
	}
	if err := file.Sync(); err != nil {
		return undo(err)
	}
	if _, err := file.WriteAt(newToc, int64(tocOffset)); err != nil {
		return undo(err)
	}
	if _, err := file.WriteAt(newHeader, 0); err != nil {
		return undo(err)
	}
	if err := file.Sync(); err != nil {
		return undo(err)
	}
	file.Close()

	// the NUP read again has the same packages, files and metadata, and the icon
	after, err := Open(path)
	if err == nil {
		err = sameWithIcon(before, after, index, icon)
	}
	if err != nil {
		return false, errors.Join(fmt.Errorf("the NUP does not read the same with the icon, so it is put back as it was: %w", err), restoreIcon(path))
	}
	if err := os.Remove(IconBackup(path)); err != nil {
		return false, err
	}
	return true, nil
}

// sameWithIcon tells whether a NUP read again after SetIcon is the one it was, with the icon.
func sameWithIcon(before *Container, after *Container, index int, icon []byte) error {
	if len(before.Packages) != len(after.Packages) || len(before.Files) != len(after.Files) || len(before.Dirs) != len(after.Dirs) {
		return errors.New("its packages, folders or files changed")
	}
	for i, file := range before.Files {
		if after.Files[i] != file {
			return fmt.Errorf("file %q changed", file.Name)
		}
	}
	for i, b := range before.Packages {
		a := after.Packages[i]
		if a.TitleID != b.TitleID || a.Kind != b.Kind || a.Version != b.Version || a.ZeroBased != b.ZeroBased {
			return fmt.Errorf("package %d changed", i)
		}
		for kind, value := range b.Meta {
			if i == index && kind == tlvIcon {
				continue
			}
			if !bytes.Equal(a.Meta[kind], value) {
				return fmt.Errorf("the metadata of package %d changed", i)
			}
		}
	}
	if !bytes.Equal(after.Packages[index].Icon(), icon) {
		return errors.New("its icon does not read back")
	}
	return nil
}

// restoreIcon puts back the header and the table of contents a copy beside a NUP keeps, and the
// size the NUP had, and removes the copy; nothing when there is none. A copy of another NUP is
// left where it is, and the work stops.
func restoreIcon(path string) error {
	data, err := os.ReadFile(IconBackup(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	le := binary.LittleEndian
	other := fmt.Errorf("%s is a copy of the table of contents of another NUP: remove it to go on", IconBackup(path))
	const fixed = 8 + 8 + 8 + 32 + headerSize + 8
	if len(data) < fixed || string(data[:8]) != iconBackupMagic {
		return other
	}
	oldSize, newSize := int64(le.Uint64(data[8:])), int64(le.Uint64(data[16:]))
	newHash := data[24:56]
	header := data[56 : 56+headerSize]
	tocOffset := le.Uint64(data[56+headerSize:])
	toc := data[fixed:]
	if oldSize <= 0 || newSize < oldSize || info.Size() < oldSize || info.Size() > newSize ||
		tocOffset+uint64(len(toc)) > uint64(oldSize) || string(header[:4]) != magic {
		return other
	}
	// the NUP is the one of the copy: as it was, or with the header or the table of contents of
	// the icon written already
	current := make([]byte, headerSize)
	if _, err := io.ReadFull(io.NewSectionReader(file, 0, headerSize), current); err != nil {
		return err
	}
	if !bytes.Equal(current[0x18:0x38], header[0x18:0x38]) && !bytes.Equal(current[0x18:0x38], newHash) {
		return other
	}
	if _, err := file.WriteAt(toc, int64(tocOffset)); err != nil {
		return err
	}
	if _, err := file.WriteAt(header, 0); err != nil {
		return err
	}
	if err := file.Truncate(oldSize); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return os.Remove(IconBackup(path))
}
