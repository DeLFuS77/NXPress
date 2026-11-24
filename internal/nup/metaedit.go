package nup

// Editing the title name and the icon of a NUP where it is, in Rename (#144). Brought from the
// edition of QuickWick (PR #151).

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image/jpeg"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxTitleBytes is the longest a title name can be: the field of a name in a NACP is 0x200 bytes,
// with the zero that ends it.
const maxTitleBytes = 0x200 - 1

// CheckTitle tells whether a text can be the title name of a game, and gives it as it is written: no
// space around it, valid text, no control character, and no more than a NACP can hold.
func CheckTitle(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", errors.New("the title name is empty")
	case !utf8.ValidString(name):
		return "", errors.New("the title name is not valid text")
	case len(name) > maxTitleBytes:
		return "", fmt.Errorf("the title name is %d bytes: a game's name is at most %d", len(name), maxTitleBytes)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			return "", errors.New("the title name has a control character")
		}
	}
	return name, nil
}

// EditMeta changes, in the metadata of the package of the game of a NUP, its title name and/or its
// icon (a cover, CheckCover): an empty name or no icon leaves that one as it is.
//
// The name is written where a NUP keeps it: its title record, the names by language (each language
// that has one) and the names of its NACP (each that has one). The icon becomes the one icon of the
// NUP, as an icon given to it is: the icons by language of its control data go, so that no emulator
// shows an old one beside it. Nothing of the files changes: as SetIcon does, the metadata of the
// package is written again after the end of the NUP, the table of contents points to it, and the
// header and the table of contents as they were are kept beside the NUP while it is written and
// put back when the NUP does not read the same afterwards (or when a work that stopped left them).
func EditMeta(path string, name string, icon []byte) error {
	if name == "" && len(icon) == 0 {
		return errors.New("there is nothing to change")
	}
	if name != "" {
		var err error
		if name, err = CheckTitle(name); err != nil {
			return err
		}
	}
	if len(icon) > 0 {
		if err := CheckCover(icon); err != nil {
			return err
		}
	}
	if err := restoreIcon(path); err != nil {
		return err
	}
	if err := restoreMeta(path); err != nil {
		return err
	}
	before, err := Open(path)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	le := binary.LittleEndian
	header := make([]byte, headerSize)
	if _, err := file.ReadAt(header, 0); err != nil {
		return err
	}
	tocOffset, tocSize := le.Uint64(header[0x08:]), le.Uint64(header[0x10:])
	toc := make([]byte, tocSize)
	if _, err := file.ReadAt(toc, int64(tocOffset)); err != nil {
		return err
	}

	// the metadata of the package of the game, and the NACP of each update that carries one: an
	// emulator shows the name of the control of the update it starts (Lichii), so a new name goes
	// in those too
	index := before.IconPackage()
	type edit struct {
		index   int
		block   []byte
		changed map[uint16]bool
	}
	var edits []edit
	for i, pkg := range before.Packages {
		if i != index && (name == "" || len(pkg.Meta[tlvNacp]) == 0) {
			continue
		}
		old := make([]byte, pkg.metaSize)
		if _, err := file.ReadAt(old, int64(pkg.metaOffset)); err != nil {
			return err
		}
		packageIcon := icon
		if i != index {
			packageIcon = nil
		}
		block, changed, err := editedMeta(old, pkg.ZeroBased, name, packageIcon, i != index)
		if err != nil {
			return err
		}
		edits = append(edits, edit{i, block, changed})
	}

	oldSize := before.FileSize
	newToc := bytes.Clone(toc)
	changed := map[int]map[uint16]bool{}
	// each block starts on 16 bytes, after the end of the NUP
	var tail bytes.Buffer
	for _, e := range edits {
		if pad := (16 - (oldSize+int64(tail.Len()))%16) % 16; pad != 0 {
			tail.Write(make([]byte, pad))
		}
		entry := newToc[tocHeadSize+e.index*packageSize:]
		le.PutUint64(entry[0x20:], uint64(oldSize+int64(tail.Len())))
		le.PutUint32(entry[0x28:], uint32(len(e.block)))
		tail.Write(e.block)
		changed[e.index] = e.changed
	}
	end := oldSize + int64(tail.Len())
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
		return fmt.Errorf("the copy of its table of contents cannot be written beside it: %w", err)
	}
	undo := func(err error) error {
		file.Close()
		return errors.Join(err, restoreIcon(path))
	}
	if _, err := file.WriteAt(tail.Bytes(), oldSize); err != nil {
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

	// the NUP read again has the same packages, files and metadata, but for what was changed
	after, err := Open(path)
	if err == nil {
		err = sameWithEdit(before, after, index, changed, name, icon)
	}
	if err != nil {
		return errors.Join(fmt.Errorf("the NUP does not read the same with the change, so it is put back as it was: %w", err), restoreIcon(path))
	}
	return os.Remove(IconBackup(path))
}

// editedMeta is the metadata block of a package with a new name and/or icon, and the types it
// changed, as the NUP is read by: the records as they were, but for the ones written again. With
// nacpOnly only the names of its NACP change, as for an update.
func editedMeta(old []byte, zeroBased bool, name string, icon []byte, nacpOnly bool) ([]byte, map[uint16]bool, error) {
	le := binary.LittleEndian
	// the type of a record as the block counts it
	stored := func(kind uint16) uint16 {
		if zeroBased {
			return kind - 1
		}
		return kind
	}
	type record struct {
		kind uint16
		raw  []byte
		data []byte
	}
	var records []record
	for pos := 0; pos+tlvHeadSize <= len(old); {
		size := int(le.Uint32(old[pos+4:]))
		if size < 0 || size > len(old)-pos-tlvHeadSize {
			return nil, nil, errors.New("a record of its metadata runs past the end of its block")
		}
		records = append(records, record{le.Uint16(old[pos:]), old[pos : pos+tlvHeadSize+size], old[pos+tlvHeadSize : pos+tlvHeadSize+size]})
		pos += tlvHeadSize + size
	}
	find := func(kind uint16) []byte {
		for _, r := range records {
			if r.kind == stored(kind) {
				return r.data
			}
		}
		return nil
	}

	// what is written again, by type as stored, and what goes; changed tells the same by the types
	// the NUP is read by, to tell that nothing else moved
	written := map[uint16][]byte{}
	gone := map[uint16]bool{}
	changed := map[uint16]bool{}
	if name != "" && !nacpOnly {
		written[stored(tlvTitleName)] = []byte(name)
		changed[tlvTitleName] = true
	}
	if name != "" {
		if titles := find(tlvLocalizedTitles); titles != nil && !nacpOnly {
			renamed, err := retitle(titles, name)
			if err != nil {
				return nil, nil, err
			}
			written[stored(tlvLocalizedTitles)] = renamed
			changed[tlvLocalizedTitles] = true
		}
		if nacp := find(tlvNacp); len(nacp) >= nacpLanguages*nacpEntry {
			written[stored(tlvNacp)] = renamedNacp(nacp, name)
			changed[tlvNacp] = true
		}
	}
	if len(icon) > 0 {
		written[stored(tlvIcon)] = icon
		gone[stored(tlvLocalizedIcons)] = true
		changed[tlvIcon], changed[tlvLocalizedIcons] = true, true
	}

	var block bytes.Buffer
	for _, r := range records {
		if _, again := written[r.kind]; again || gone[r.kind] {
			continue
		}
		block.Write(r.raw)
	}
	kinds := make([]int, 0, len(written))
	for kind := range written {
		kinds = append(kinds, int(kind))
	}
	sort.Ints(kinds)
	for _, kind := range kinds {
		data := written[uint16(kind)]
		var head [tlvHeadSize]byte
		le.PutUint16(head[0:], uint16(kind))
		le.PutUint32(head[4:], uint32(len(data)))
		block.Write(head[:])
		block.Write(data)
	}
	if block.Len() > maxMetaSize {
		return nil, nil, errors.New("its metadata would be too big")
	}
	return block.Bytes(), changed, nil

}

// renamedNacp is a NACP with a new name in each language that has one.
func renamedNacp(nacp []byte, name string) []byte {
	patched := bytes.Clone(nacp)
	if len(patched) < nacpLanguages*nacpEntry {
		return patched
	}
	for i := 0; i < nacpLanguages; i++ {
		field := patched[i*nacpEntry : i*nacpEntry+0x200]
		// only the languages that have a name get the new one
		if len(cString(field)) > 0 {
			clear(field)
			copy(field, name)
		}
	}
	return patched
}

// retitle writes the names by language of a package again, each language it has with the new name,
// as it was written: the tags as they are, in their order.
func retitle(payload []byte, name string) ([]byte, error) {
	var out []byte
	for pos := 0; pos < len(payload); {
		tagSize := int(payload[pos])
		pos++
		if tagSize == 0 || tagSize > len(payload)-pos-4 {
			return nil, errors.New("a localized record is malformed")
		}
		tag := payload[pos : pos+tagSize]
		pos += tagSize
		size := int(binary.LittleEndian.Uint32(payload[pos:]))
		pos += 4
		if size < 0 || size > len(payload)-pos {
			return nil, errors.New("a localized record runs past the end of its block")
		}
		pos += size
		out = append(out, byte(len(tag)))
		out = append(out, tag...)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(name)))
		out = append(out, name...)
	}
	return out, nil
}

// sameWithEdit tells whether a NUP read again after EditMeta is the one it was, with what was
// changed: the same packages, folders, files and metadata, the new name read back as the name of the
// package, and the new icon as its only icon.
func sameWithEdit(before *Container, after *Container, index int, changed map[int]map[uint16]bool, name string, icon []byte) error {
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
			if changed[i][kind] {
				continue
			}
			if !bytes.Equal(a.Meta[kind], value) {
				return fmt.Errorf("the metadata of package %d changed", i)
			}
		}
	}
	if name != "" {
		if want := cleanName([]byte(name)); want != "" && after.Packages[index].Name() != want {
			return errors.New("its name does not read back")
		}
	}
	if len(icon) > 0 {
		if !bytes.Equal(after.Packages[index].Icon(), icon) {
			return errors.New("its icon does not read back")
		}
		if len(after.Packages[index].Meta[tlvLocalizedIcons]) != 0 {
			return errors.New("the icons by language are still there")
		}
	}
	return nil
}

// CheckCover tells whether data is an icon a NUP can be given: a JPEG of IconSize × IconSize pixels
// that reads whole, no bigger than the icons of a control.
func CheckCover(data []byte) error {
	if len(data) == 0 {
		return errors.New("the icon is empty")
	}
	if len(data) > maxMetaSize/32 {
		return fmt.Errorf("the icon is %d KiB: the icon of a game is at most %d KiB", (len(data)+1023)>>10, maxMetaSize/32>>10)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("the icon is not a JPEG image: %w", err)
	}
	if size := img.Bounds().Size(); size.X != IconSize || size.Y != IconSize {
		return fmt.Errorf("the icon is %d × %d pixels, not %d × %d", size.X, size.Y, IconSize, IconSize)
	}
	return nil
}
