// Package testnup builds small NUP files (containers of decrypted games) for tests: a header, a
// table of contents with the tree of files of each package, the data of the files, raw or
// compressed with zstd, then the metadata of each package, as the format
// (github.com/a55uka/nup, version 1) describes them. It is the builder of the library app
// (internal/testnup of Switch Library Manager Web), with files in the packages.
package testnup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	headerSize  = 0x80
	tocHeadSize = 0x30
	packageSize = 0x40
	dirSize     = 0x18
	fileSize    = 0x50
	noDir       = 0xFFFFFFFF
	fileTag     = 0x80000000
)

// kinds of packages
const (
	Base   = 0
	Update = 1
	Addon  = 2
)

// types of metadata records
const (
	Nacp            = 0x0002
	Icon            = 0x0003
	DisplayVersion  = 0x0004
	TitleName       = 0x0005
	Publisher       = 0x0006
	LocalizedTitles = 0x0009
	LocalizedIcons  = 0x000A
)

// Record is one record of the metadata of a package.
type Record struct {
	Type    uint16
	Payload []byte
}

// File is a file of a package, by its path in it ("romfs/Data/a.bin"), stored compressed with
// zstd when Zstd is set.
type File struct {
	Path string
	Data []byte
	Zstd bool
}

// Package is one package of a NUP.
type Package struct {
	TitleId uint64
	Kind    byte
	Flags   byte
	Version uint32
	Meta    []Record
	Files   []File
}

// Options change how a NUP is written, as some packers write it.
type Options struct {
	// the files among the children of a folder tagged with the high bit, as some builds of the
	// packer of the format write them, instead of counted after the folders
	TagFiles bool
	// files with the same data keep one copy of it, as the packer of the format does
	Dedup bool
	// bytes left between the header and the table of contents, which the format allows
	TocGap int
}

// Localized is the payload of a record of names or icons by language: tag, data, tag, data...
func Localized(entries ...string) []byte {
	var out bytes.Buffer
	for i := 0; i+1 < len(entries); i += 2 {
		out.WriteByte(byte(len(entries[i])))
		out.WriteString(entries[i])
		binary.Write(&out, binary.LittleEndian, uint32(len(entries[i+1])))
		out.WriteString(entries[i+1])
	}
	return out.Bytes()
}

// Jpeg is a made-up icon that starts as a JPEG image does.
var Jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{7}, 60)...)

// Build writes a NUP with packages, files of the same data stored once.
func Build(packages []Package) []byte {
	return BuildWith(packages, Options{Dedup: true})
}

type node struct {
	name     string
	children map[string]*node
	file     int
	index    uint32
}

// BuildWith writes a NUP with packages, as the options say.
func BuildWith(packages []Package, o Options) []byte {
	le := binary.LittleEndian
	n := len(packages)

	// the trees: a root per package, its folders, then its files, in the order of their names
	var dirs []*node
	type fileRecord struct {
		name  string
		data  []byte
		zstd  bool
		index int
	}
	var files []fileRecord
	roots := make([]*node, n)
	exefs := make([]uint32, n)
	romfs := make([]uint32, n)
	for i, pkg := range packages {
		root := &node{children: map[string]*node{}, file: -1}
		roots[i] = root
		for _, file := range pkg.Files {
			parts := strings.Split(file.Path, "/")
			at := root
			for _, part := range parts[:len(parts)-1] {
				next, ok := at.children[part]
				if !ok {
					next = &node{name: part, children: map[string]*node{}, file: -1}
					at.children[part] = next
				}
				at = next
			}
			at.children[parts[len(parts)-1]] = &node{name: parts[len(parts)-1], file: len(files)}
			files = append(files, fileRecord{name: parts[len(parts)-1], data: file.Data, zstd: file.Zstd, index: len(files)})
		}
	}
	// folders numbered in the order of the tree, each package after the one before
	var order func(at *node)
	order = func(at *node) {
		at.index = uint32(len(dirs))
		dirs = append(dirs, at)
		for _, child := range sortedChildren(at) {
			if child.file < 0 {
				order(child)
			}
		}
	}
	for i, root := range roots {
		order(root)
		exefs[i], romfs[i] = noDir, noDir
		if child, ok := root.children["exefs"]; ok && child.file < 0 {
			exefs[i] = child.index
		}
		if child, ok := root.children["romfs"]; ok && child.file < 0 {
			romfs[i] = child.index
		}
	}
	var pool bytes.Buffer
	names := map[string]uint32{}
	intern := func(name string) (uint32, uint32) {
		if at, ok := names[name]; ok {
			return at, uint32(len(name))
		}
		at := uint32(pool.Len())
		names[name] = at
		pool.WriteString(name)
		return at, uint32(len(name))
	}
	var children []uint32
	firsts := make([]uint32, len(dirs))
	for i, dir := range dirs {
		firsts[i] = uint32(len(children))
		for _, child := range sortedChildren(dir) {
			switch {
			case child.file < 0:
				children = append(children, child.index)
			case o.TagFiles:
				children = append(children, fileTag+uint32(child.file))
			default:
				children = append(children, uint32(len(dirs))+uint32(child.file))
			}
		}
	}

	records := tocHeadSize + n*packageSize + len(dirs)*dirSize + len(files)*fileSize
	poolOffset := records + len(children)*4
	for _, dir := range dirs {
		intern(dir.name)
	}
	for _, file := range files {
		intern(file.name)
	}
	tocSize := poolOffset + pool.Len()
	tocOffset := headerSize + o.TocGap
	blobOffset := (tocOffset + tocSize + 15) &^ 15

	// the data of the files, each aligned, the same data stored once
	type extent struct {
		offset, size int
		codec        byte
	}
	extents := make([]extent, len(files))
	var blob bytes.Buffer
	seen := map[[32]byte]extent{}
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	for i, file := range files {
		if len(file.data) == 0 {
			continue
		}
		hash := sha256.Sum256(append([]byte{boolByte(file.zstd)}, file.data...))
		if known, ok := seen[hash]; ok && o.Dedup {
			extents[i] = known
			continue
		}
		stored, codec := file.data, byte(0)
		if file.zstd {
			stored, codec = encoder.EncodeAll(file.data, nil), 1
		}
		for (blobOffset+blob.Len())%16 != 0 {
			blob.WriteByte(0)
		}
		extents[i] = extent{offset: blobOffset + blob.Len(), size: len(stored), codec: codec}
		seen[hash] = extents[i]
		blob.Write(stored)
	}
	encoder.Close()

	// the metadata of each package, after the data
	end := blobOffset + blob.Len()
	blocks := make([][]byte, n)
	offsets := make([]int, n)
	for i, pkg := range packages {
		var block bytes.Buffer
		for _, record := range pkg.Meta {
			binary.Write(&block, le, record.Type)
			binary.Write(&block, le, uint16(0))
			binary.Write(&block, le, uint32(len(record.Payload)))
			block.Write(record.Payload)
		}
		blocks[i] = block.Bytes()
		if len(blocks[i]) > 0 {
			end = (end + 15) &^ 15
			offsets[i] = end
			end += len(blocks[i])
		}
	}

	toc := make([]byte, tocSize)
	copy(toc, "TOC\x01")
	le.PutUint32(toc[0x04:], uint32(n))
	le.PutUint64(toc[0x08:], uint64(len(dirs)))
	le.PutUint64(toc[0x10:], uint64(len(files)))
	le.PutUint64(toc[0x18:], uint64(poolOffset))
	le.PutUint64(toc[0x20:], uint64(pool.Len()))
	for i, pkg := range packages {
		record := toc[tocHeadSize+i*packageSize:]
		le.PutUint64(record[0x00:], pkg.TitleId)
		le.PutUint32(record[0x08:], roots[i].index)
		le.PutUint32(record[0x0C:], exefs[i])
		le.PutUint32(record[0x10:], romfs[i])
		le.PutUint32(record[0x14:], pkg.Version)
		record[0x18] = pkg.Kind
		record[0x19] = pkg.Flags
		if len(blocks[i]) > 0 {
			le.PutUint64(record[0x20:], uint64(offsets[i]))
			le.PutUint32(record[0x28:], uint32(len(blocks[i])))
		}
	}
	position := tocHeadSize + n*packageSize
	for i, dir := range dirs {
		at, length := intern(dir.name)
		le.PutUint32(toc[position:], at)
		le.PutUint32(toc[position+4:], length)
		le.PutUint32(toc[position+8:], firsts[i])
		le.PutUint32(toc[position+12:], uint32(len(dir.children)))
		position += dirSize
	}
	for i, file := range files {
		at, length := intern(file.name)
		hash := sha256.Sum256(file.data)
		le.PutUint32(toc[position:], at)
		le.PutUint32(toc[position+4:], length)
		le.PutUint64(toc[position+0x08:], uint64(len(file.data)))
		le.PutUint64(toc[position+0x10:], uint64(extents[i].offset))
		le.PutUint64(toc[position+0x18:], uint64(extents[i].size))
		copy(toc[position+0x20:], hash[:])
		toc[position+0x40] = extents[i].codec
		position += fileSize
	}
	for _, child := range children {
		le.PutUint32(toc[position:], child)
		position += 4
	}
	copy(toc[poolOffset:], pool.Bytes())

	file := make([]byte, end)
	copy(file, "NUP\x01")
	le.PutUint16(file[0x04:], headerSize)
	le.PutUint16(file[0x06:], 1)
	le.PutUint64(file[0x08:], uint64(tocOffset))
	le.PutUint64(file[0x10:], uint64(tocSize))
	le.PutUint64(file[0x38:], uint64(blobOffset))
	le.PutUint64(file[0x40:], uint64(end))
	le.PutUint32(file[0x48:], uint32(n))
	copy(file[tocOffset:], toc)
	copy(file[blobOffset:], blob.Bytes())
	for i, block := range blocks {
		copy(file[offsets[i]:], block)
	}
	Rehash(file)
	return file
}

// sortedChildren are the children of a folder in the order of their names, as the format asks.
func sortedChildren(at *node) []*node {
	result := make([]*node, 0, len(at.children))
	for _, child := range at.children {
		result = append(result, child)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// Rehash writes again the hash of the table of contents of a NUP whose table was changed.
func Rehash(file []byte) {
	tocOffset := binary.LittleEndian.Uint64(file[0x08:])
	tocSize := binary.LittleEndian.Uint64(file[0x10:])
	hash := sha256.Sum256(file[tocOffset : tocOffset+tocSize])
	copy(file[0x18:], hash[:])
}

// Extent tells where the data of the file at path of a package is in a NUP built from packages,
// and its size as stored, to damage it in a test.
func Extent(nup []byte, packages []Package, pkg int, path string) (offset int, size int) {
	le := binary.LittleEndian
	index := 0
	for i := 0; i < pkg; i++ {
		index += len(packages[i].Files)
	}
	for _, file := range packages[pkg].Files {
		if file.Path == path {
			break
		}
		index++
	}
	n := len(packages)
	toc := nup[le.Uint64(nup[0x08:]):]
	dirCount := int(le.Uint64(toc[0x08:]))
	record := toc[tocHeadSize+n*packageSize+dirCount*dirSize+index*fileSize:]
	return int(le.Uint64(record[0x10:])), int(le.Uint64(record[0x18:]))
}
