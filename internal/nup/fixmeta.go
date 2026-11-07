package nup

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// The metadata of a NUP whose types a build of the packer of the format counted from 0 is
// written again with them where the format puts them, for Eglantine and Lichii, which read them
// as they are written. Only the type of each record changes, two bytes each, in the blocks of
// metadata at the end of the NUP: no hash covers them, the table of contents and the data of the
// files stay as they are. The blocks as they were are kept beside the NUP while they are
// written, and put back when the NUP does not read the same afterwards; a NUP left with such a
// copy by a work that stopped gets its blocks back before anything else.

// MetaBackup is the copy of the blocks of metadata of a NUP kept while they are written again.
func MetaBackup(path string) string {
	return path + ".nxpress-meta"
}

const backupMagic = "NXPMETA1"

// FixZeroBased writes the types of the metadata of the packages of a NUP counted from 0 where the
// format puts them, and gives how many packages it wrote again: 0 when none had them so.
func FixZeroBased(path string) (int, error) {
	if err := restoreMeta(path); err != nil {
		return 0, err
	}
	before, err := Open(path)
	if err != nil {
		return 0, err
	}
	type block struct {
		offset uint64
		old    []byte
		fixed  []byte
	}
	blocks := []block{}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	for _, pkg := range before.Packages {
		if !pkg.ZeroBased {
			continue
		}
		old := make([]byte, pkg.metaSize)
		if _, err := file.ReadAt(old, int64(pkg.metaOffset)); err != nil {
			return 0, err
		}
		fixed, err := countFromOne(old)
		if err != nil {
			return 0, err
		}
		blocks = append(blocks, block{pkg.metaOffset, old, fixed})
	}
	if len(blocks) == 0 {
		return 0, nil
	}

	// the blocks as they were, beside the NUP, with what tells the NUP they belong to
	header := make([]byte, headerSize)
	if _, err := file.ReadAt(header, 0); err != nil {
		return 0, err
	}
	var backup bytes.Buffer
	backup.WriteString(backupMagic)
	binary.Write(&backup, binary.LittleEndian, uint64(before.FileSize))
	backup.Write(header[0x18:0x38])
	binary.Write(&backup, binary.LittleEndian, uint32(len(blocks)))
	for _, b := range blocks {
		binary.Write(&backup, binary.LittleEndian, b.offset)
		binary.Write(&backup, binary.LittleEndian, uint32(len(b.old)))
		backup.Write(b.old)
	}
	if err := writeSynced(MetaBackup(path), backup.Bytes()); err != nil {
		return 0, fmt.Errorf("the copy of its metadata cannot be written beside it: %w", err)
	}

	for _, b := range blocks {
		if _, err := file.WriteAt(b.fixed, int64(b.offset)); err != nil {
			file.Close()
			return 0, errors.Join(err, restoreMeta(path))
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return 0, errors.Join(err, restoreMeta(path))
	}
	file.Close()

	// the NUP read again shows the same names, publishers and versions, counted from 1 now
	after, err := Open(path)
	if err == nil {
		err = sameMeta(before, after)
	}
	if err != nil {
		return 0, errors.Join(fmt.Errorf("the NUP does not read the same with its metadata written again, so it is put back as it was: %w", err), restoreMeta(path))
	}
	if err := os.Remove(MetaBackup(path)); err != nil {
		return 0, err
	}
	return len(blocks), nil
}

// countFromOne is a block of metadata with each type one up, as parseTlvs reads it.
func countFromOne(block []byte) ([]byte, error) {
	fixed := bytes.Clone(block)
	for pos := 0; pos+tlvHeadSize <= len(fixed); {
		kind := binary.LittleEndian.Uint16(fixed[pos:])
		size := int(binary.LittleEndian.Uint32(fixed[pos+4:]))
		if size < 0 || size > len(fixed)-pos-tlvHeadSize {
			return nil, errors.New("a record of its metadata runs past the end of its block")
		}
		if kind < 0x00FF {
			binary.LittleEndian.PutUint16(fixed[pos:], kind+1)
		}
		pos += tlvHeadSize + size
	}
	return fixed, nil
}

// sameMeta tells whether two readings of a NUP give the same metadata, the second counted from 1.
func sameMeta(before *Container, after *Container) error {
	if len(before.Packages) != len(after.Packages) {
		return errors.New("its packages changed")
	}
	for i, a := range after.Packages {
		b := before.Packages[i]
		if a.ZeroBased {
			return fmt.Errorf("package %d is still counted from 0", i)
		}
		if a.Name() != b.Name() || a.Publisher() != b.Publisher() || a.DisplayVersion() != b.DisplayVersion() ||
			!bytes.Equal(a.Icon(), b.Icon()) || len(a.Meta) != len(b.Meta) {
			return fmt.Errorf("package %d reads otherwise", i)
		}
		for kind, value := range b.Meta {
			if !bytes.Equal(a.Meta[kind], value) {
				return fmt.Errorf("package %d reads otherwise", i)
			}
		}
	}
	return nil
}

// restoreMeta puts back the blocks a copy beside a NUP keeps, and removes the copy; nothing when
// there is none. A copy of another NUP (of another size or table of contents) is left where it
// is, and the work stops.
func restoreMeta(path string) error {
	data, err := os.ReadFile(MetaBackup(path))
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
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(io.NewSectionReader(file, 0, headerSize), header); err != nil {
		return err
	}
	le := binary.LittleEndian
	other := fmt.Errorf("%s is a copy of the metadata of another NUP: remove it to go on", MetaBackup(path))
	if len(data) < 8+8+32+4 || string(data[:8]) != backupMagic || int64(le.Uint64(data[8:])) != info.Size() || !bytes.Equal(data[16:48], header[0x18:0x38]) {
		return other
	}
	count, pos := int(le.Uint32(data[48:])), 52
	for range count {
		if pos+12 > len(data) {
			return other
		}
		offset, size := le.Uint64(data[pos:]), int(le.Uint32(data[pos+8:]))
		pos += 12
		if size > len(data)-pos || offset > uint64(info.Size()) || uint64(size) > uint64(info.Size())-offset {
			return other
		}
		if _, err := file.WriteAt(data[pos:pos+size], int64(offset)); err != nil {
			return err
		}
		pos += size
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return os.Remove(MetaBackup(path))
}

// writeSynced writes a new file and makes sure it is on the disk.
func writeSynced(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
