// Package nup reads NUP files (github.com/a55uka/nup, version 1): containers of decrypted games
// of the Switch and the Switch 2, a game with its updates and its DLC in one file, as plain
// ExeFS and RomFS trees, each file stored raw or compressed with zstd, with its own SHA-256.
// Nothing in them is encrypted, so they are read and verified without the keys of the console.
// NXPress shows and verifies them; it does not compress, convert or repack them.
//
// The header, the table of contents and the metadata of the packages are read as the library
// app reads them (switchfs/nup.go of Switch Library Manager Web), with the same checks; this
// reader reads the folders and the files of the table too, and the data of the files to check it.
package nup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	magic       = "NUP\x01"
	tocMagic    = "TOC\x01"
	headerSize  = 0x80
	tocHeadSize = 0x30
	packageSize = 0x40
	dirSize     = 0x18
	fileSize    = 0x50
	tlvHeadSize = 8

	// NoDir is a folder a package does not have (its ExeFS or its RomFS)
	NoDir = 0xFFFFFFFF
	// some builds of the packer of the format tag the files among the children of a folder with
	// the high bit, instead of counting them after the folders as the format says: both are read
	fileTag = 0x80000000

	// far above any real file: a damaged or hostile one is refused before it is read whole. The
	// largest known game has 372587 files, a table of 34 MB.
	maxTocSize  = 256 << 20
	maxPackages = 4096
	maxMetaSize = 32 << 20
	// all the metadata of a file together: a few icons and names, not the gigabytes a file that
	// points every package at the same big block would make it read
	maxMetaTotal = 64 << 20
	// folders inside folders: a RomFS goes a few levels deep, a loop would go on for ever
	maxDepth = 128
)

// kinds of the packages of a NUP; 3 and 4 are reserved by the format
const (
	KindBase   = 0
	KindUpdate = 1
	KindAddon  = 2
	kindLast   = 4
)

// codecs of the data of a file
const (
	CodecRaw  = 0
	CodecZstd = 1
)

// the types of the metadata of a package (TLV records)
const (
	tlvNacp            = 0x0002
	tlvIcon            = 0x0003
	tlvDisplayVersion  = 0x0004
	tlvTitleName       = 0x0005
	tlvPublisher       = 0x0006
	tlvLocalizedTitles = 0x0009
	tlvLocalizedIcons  = 0x000A
)

// Package is one package of a NUP: the game, one of its updates or one of its DLC.
type Package struct {
	// as stored: the ID of the program for the game and its updates, the ID of the DLC for a DLC
	TitleID uint64
	Kind    byte
	// bit 0: an update stored without its game
	Flags   byte
	Version uint32
	// the folders of the package: its root, and its ExeFS and RomFS (NoDir when it has none)
	RootDir  uint32
	ExefsDir uint32
	RomfsDir uint32
	// the metadata of the package by type, the first record of each type
	Meta map[uint16][]byte
}

// Dir is a folder of the tree of a package.
type Dir struct {
	Name       string
	FirstChild uint32
	ChildCount uint32
}

// File is a file of the tree of a package, with where its data is stored.
type File struct {
	Name string
	// its size uncompressed, and the place and the size of its data in the NUP
	Size         uint64
	ExtentOffset uint64
	ExtentSize   uint64
	// the SHA-256 of the file uncompressed
	Hash  [32]byte
	Codec byte
}

// Container is what the header and the table of contents of a NUP tell.
type Container struct {
	// the size of the NUP itself
	FileSize int64
	Packages []Package
	Dirs     []Dir
	Files    []File
	// the children of the folders, each a node: a folder below len(Dirs), else the file at
	// node - len(Dirs)
	Children []uint32
}

// IsNup tells a NUP by the extension of its name.
func IsNup(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".nup")
}

// Open reads the header, the table of contents and the metadata of the packages of a NUP, never
// the data of its files.
func Open(path string) (*Container, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return Read(file, info.Size())
}

// Read checks a NUP as the format asks of a reader before it trusts anything of it: the sizes
// and the offsets inside the file, the hash of the table of contents, no unknown flags, kinds or
// codecs, every name, folder and file inside its region.
func Read(r io.ReaderAt, size int64) (*Container, error) {
	header := make([]byte, headerSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("NUP header: %w", err)
	}
	if string(header[:4]) != magic {
		if string(header[:3]) == "NUP" {
			return nil, fmt.Errorf("NUP version %d is not supported", header[3])
		}
		return nil, errors.New("not a NUP file")
	}
	le := binary.LittleEndian
	headerLength := uint64(le.Uint16(header[0x04:]))
	tocOffset := le.Uint64(header[0x08:])
	tocSize := le.Uint64(header[0x10:])
	tocHash := header[0x18:0x38]
	blobOffset := le.Uint64(header[0x38:])
	total := le.Uint64(header[0x40:])
	packageCount := uint64(le.Uint32(header[0x48:]))
	flags := le.Uint32(header[0x4C:])

	switch {
	case headerLength < headerSize:
		return nil, errors.New("NUP header is too small")
	case flags != 0:
		return nil, fmt.Errorf("NUP flags %#x are not supported", flags)
	case tocOffset != headerLength:
		return nil, errors.New("NUP table of contents is not after the header")
	case tocSize < tocHeadSize || tocSize > maxTocSize:
		return nil, fmt.Errorf("NUP table of contents has an invalid size (%d bytes)", tocSize)
	case packageCount == 0 || packageCount > maxPackages:
		return nil, fmt.Errorf("NUP has an invalid number of packages (%d)", packageCount)
	case tocHeadSize+packageCount*packageSize > tocSize:
		return nil, errors.New("NUP packages do not fit in the table of contents")
	case blobOffset%16 != 0 || blobOffset < tocOffset+tocSize:
		return nil, errors.New("NUP data region overlaps the table of contents")
	}
	if uint64(size) < total {
		return nil, cutShort{size: size, total: total}
	}
	if uint64(size) != total || blobOffset > total {
		return nil, fmt.Errorf("NUP file size %d does not match its header (%d)", size, total)
	}

	toc := make([]byte, tocSize)
	if _, err := r.ReadAt(toc, int64(tocOffset)); err != nil {
		return nil, fmt.Errorf("NUP table of contents: %w", err)
	}
	if sum := sha256.Sum256(toc); !bytes.Equal(sum[:], tocHash) {
		return nil, errors.New("NUP table of contents is damaged (its hash does not match)")
	}
	if string(toc[:4]) != tocMagic {
		return nil, errors.New("NUP table of contents has an unknown format")
	}
	if uint64(le.Uint32(toc[0x04:])) != packageCount {
		return nil, errors.New("NUP table of contents disagrees with the header")
	}
	dirCount := le.Uint64(toc[0x08:])
	fileCount := le.Uint64(toc[0x10:])
	poolOffset := le.Uint64(toc[0x18:])
	poolSize := le.Uint64(toc[0x20:])
	if dirCount > tocSize/dirSize || fileCount > tocSize/fileSize {
		return nil, errors.New("NUP folders and files do not fit in the table of contents")
	}
	records := tocHeadSize + packageCount*packageSize + dirCount*dirSize + fileCount*fileSize
	switch {
	case records > tocSize:
		return nil, errors.New("NUP folders and files do not fit in the table of contents")
	case poolOffset < records || poolOffset > tocSize || poolSize > tocSize-poolOffset:
		return nil, errors.New("NUP names are outside the table of contents")
	case (poolOffset-records)%4 != 0:
		return nil, errors.New("NUP list of the children of the folders is malformed")
	}
	pool := toc[poolOffset : poolOffset+poolSize]
	name := func(record []byte) (string, error) {
		offset, length := uint64(le.Uint32(record)), uint64(le.Uint32(record[4:]))
		if offset+length > poolSize {
			return "", errors.New("NUP name is outside the table of contents")
		}
		return string(pool[offset : offset+length]), nil
	}
	children := (poolOffset - records) / 4

	result := &Container{FileSize: size, Packages: make([]Package, 0, packageCount),
		Dirs: make([]Dir, dirCount), Files: make([]File, fileCount), Children: make([]uint32, children)}
	metaTotal := uint64(0)
	for i := uint64(0); i < packageCount; i++ {
		record := toc[tocHeadSize+i*packageSize:][:packageSize]
		pkg := Package{
			TitleID:  le.Uint64(record[0x00:]),
			RootDir:  le.Uint32(record[0x08:]),
			ExefsDir: le.Uint32(record[0x0C:]),
			RomfsDir: le.Uint32(record[0x10:]),
			Version:  le.Uint32(record[0x14:]),
			Kind:     record[0x18],
			Flags:    record[0x19],
		}
		if pkg.Kind > kindLast {
			return nil, fmt.Errorf("NUP package %d has an unknown kind (%d)", i, pkg.Kind)
		}
		if uint64(pkg.RootDir) >= dirCount || pkg.ExefsDir != NoDir && uint64(pkg.ExefsDir) >= dirCount ||
			pkg.RomfsDir != NoDir && uint64(pkg.RomfsDir) >= dirCount {
			return nil, fmt.Errorf("NUP package %d has its folders outside the table of contents", i)
		}
		metaOffset := le.Uint64(record[0x20:])
		metaSize := uint64(le.Uint32(record[0x28:]))
		if metaSize > 0 {
			if metaSize > maxMetaSize || metaOffset < tocOffset+tocSize || metaOffset > total || metaSize > total-metaOffset {
				return nil, fmt.Errorf("NUP package %d has its metadata outside the file", i)
			}
			if metaTotal += metaSize; metaTotal > maxMetaTotal {
				return nil, errors.New("NUP metadata is too big")
			}
			block := make([]byte, metaSize)
			if _, err := r.ReadAt(block, int64(metaOffset)); err != nil {
				return nil, fmt.Errorf("NUP package %d metadata: %w", i, err)
			}
			meta, err := parseTlvs(block)
			if err != nil {
				return nil, fmt.Errorf("NUP package %d metadata: %w", i, err)
			}
			pkg.Meta = meta
		}
		result.Packages = append(result.Packages, pkg)
	}

	position := tocHeadSize + packageCount*packageSize
	for i := range result.Dirs {
		record := toc[position:][:dirSize]
		position += dirSize
		dirName, err := name(record)
		if err != nil {
			return nil, err
		}
		dir := Dir{Name: dirName, FirstChild: le.Uint32(record[0x08:]), ChildCount: le.Uint32(record[0x0C:])}
		if uint64(dir.FirstChild)+uint64(dir.ChildCount) > children {
			return nil, fmt.Errorf("NUP folder %q has its children outside the table of contents", dirName)
		}
		result.Dirs[i] = dir
	}
	for i := range result.Files {
		record := toc[position:][:fileSize]
		position += fileSize
		fileName, err := name(record)
		if err != nil {
			return nil, err
		}
		file := File{Name: fileName, Size: le.Uint64(record[0x08:]), ExtentOffset: le.Uint64(record[0x10:]),
			ExtentSize: le.Uint64(record[0x18:]), Codec: record[0x40]}
		copy(file.Hash[:], record[0x20:0x40])
		if file.Codec > CodecZstd {
			return nil, fmt.Errorf("NUP file %q is stored with codec %d, which NUP version 1 does not have", fileName, file.Codec)
		}
		// a file of 0 bytes has no data; any other has its data in the data region, aligned
		if file.Size > 0 {
			switch {
			case file.ExtentOffset < blobOffset || file.ExtentOffset%16 != 0 ||
				file.ExtentSize > total-file.ExtentOffset:
				return nil, fmt.Errorf("NUP file %q has its data outside the file", fileName)
			case file.Codec == CodecRaw && file.ExtentSize != file.Size:
				return nil, fmt.Errorf("NUP file %q is stored raw with %d bytes instead of %d", fileName, file.ExtentSize, file.Size)
			case file.Codec == CodecZstd && file.ExtentSize == 0:
				return nil, fmt.Errorf("NUP file %q has no data", fileName)
			}
		}
		result.Files[i] = file
	}
	for i := range result.Children {
		node := le.Uint32(toc[position:])
		position += 4
		if node >= fileTag {
			node = uint32(dirCount) + (node - fileTag)
		}
		if uint64(node) >= dirCount+fileCount {
			return nil, errors.New("NUP folder holds a file or a folder that is not in the table of contents")
		}
		result.Children[i] = node
	}
	return result, nil
}

// cutShort is a NUP shorter than its header says: a copy or a download that stopped before its
// end. It is an io.ErrUnexpectedEOF.
type cutShort struct {
	size  int64
	total uint64
}

func (e cutShort) Error() string {
	return fmt.Sprintf("the NUP file is cut short: %d of %d bytes, a copy or a download that stopped before its end", e.size, e.total)
}

func (e cutShort) Unwrap() error {
	return io.ErrUnexpectedEOF
}

// parseTlvs splits a block of metadata into its records; the block ends where the header of
// the next record would not fit, and a record that runs past the block is an error.
func parseTlvs(block []byte) (map[uint16][]byte, error) {
	result := map[uint16][]byte{}
	for pos := 0; pos+tlvHeadSize <= len(block); {
		kind := binary.LittleEndian.Uint16(block[pos:])
		size := int(binary.LittleEndian.Uint32(block[pos+4:]))
		pos += tlvHeadSize
		if size < 0 || size > len(block)-pos {
			return nil, errors.New("a record runs past the end of the block")
		}
		if _, seen := result[kind]; !seen {
			result[kind] = block[pos : pos+size]
		}
		pos += size
	}
	return result, nil
}

// Walk calls fn for every file of a package, with its path inside the package
// ("romfs/Data/stage.bin") and its index in Files, in the order of the tree. A folder held by
// two folders is walked once; a folder inside itself, or one too deep, is an error.
func (c *Container) Walk(pkg int, fn func(path string, file int) error) error {
	const (
		unseen = iota
		open
		walked
	)
	state := make([]byte, len(c.Dirs))
	type frame struct {
		dir    uint32
		next   uint32
		prefix string
	}
	root := c.Packages[pkg].RootDir
	stack := []frame{{dir: root}}
	state[root] = open
	dirCount := uint32(len(c.Dirs))
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		dir := c.Dirs[top.dir]
		if top.next == dir.ChildCount {
			state[top.dir] = walked
			stack = stack[:len(stack)-1]
			continue
		}
		node := c.Children[dir.FirstChild+top.next]
		top.next++
		if node >= dirCount {
			if err := fn(top.prefix+c.Files[node-dirCount].Name, int(node-dirCount)); err != nil {
				return err
			}
			continue
		}
		switch state[node] {
		case open:
			return fmt.Errorf("NUP folder %q is inside itself", c.Dirs[node].Name)
		case walked:
			continue
		}
		if len(stack) >= maxDepth {
			return errors.New("NUP folders go too deep")
		}
		state[node] = open
		stack = append(stack, frame{dir: node, prefix: top.prefix + c.Dirs[node].Name + "/"})
	}
	return nil
}

// Summary is what a package holds: its files, their size uncompressed and the size of their
// data as stored, each piece of data counted once.
type Summary struct {
	Files  int
	Size   uint64
	Stored uint64
}

// Summarize tells what a package holds.
func (c *Container) Summarize(pkg int) (Summary, error) {
	result := Summary{}
	extents := map[uint64]bool{}
	err := c.Walk(pkg, func(_ string, index int) error {
		file := c.Files[index]
		result.Files++
		result.Size += file.Size
		if file.Size > 0 && !extents[file.ExtentOffset] {
			extents[file.ExtentOffset] = true
			result.Stored += file.ExtentSize
		}
		return nil
	})
	return result, err
}

// Totals tells what the whole NUP holds: the files of all its packages, their size uncompressed,
// and the size of their data as stored, each piece kept once however many files share it.
func (c *Container) Totals() Summary {
	result := Summary{Files: len(c.Files)}
	extents := map[uint64]bool{}
	for _, file := range c.Files {
		result.Size += file.Size
		if file.Size > 0 && !extents[file.ExtentOffset] {
			extents[file.ExtentOffset] = true
			result.Stored += file.ExtentSize
		}
	}
	return result
}

// TitleIDText is the title ID of a package as the library app and the eShop give it, as text. The
// game and its updates are stored with the ID of the program (from its NPDM): an update takes
// the ID updates have in the eShop, the one of the game with 800 at the end.
func (pkg Package) TitleIDText() string {
	id := pkg.TitleID
	if pkg.Kind == KindUpdate && id&0x800 == 0 {
		id = id&^0xFFF | 0x800
	}
	return fmt.Sprintf("%016X", id)
}

// KindName is the kind of a package as NXPress tells titles: BASE, UPD or DLC.
func (pkg Package) KindName() string {
	switch pkg.Kind {
	case KindBase:
		return "BASE"
	case KindUpdate:
		return "UPD"
	case KindAddon:
		return "DLC"
	}
	return fmt.Sprintf("kind %d", pkg.Kind)
}

// the NACP: 16 languages of 0x300 bytes (a name of 0x200, a publisher of 0x100), the version
// shown at 0x3060
const (
	nacpSize        = 0x4000
	nacpEntry       = 0x300
	nacpLanguages   = 16
	nacpVersionAt   = 0x3060
	nacpVersionSize = 0x10
)

// languages are the BCP-47 tags of the NUP format for each language of a NACP, in its order.
var languages = [nacpLanguages]string{"en-us", "en-gb", "ja", "fr", "de", "es-419", "es", "it", "nl", "fr-ca", "pt",
	"ru", "ko", "zh-hant", "zh-hans", "pt-br"}

// Name is the name of the game a package gives: American English, else the name of the default
// language, else the first other name; "" when it gives none.
func (pkg Package) Name() string {
	if localized, err := parseLocalized(pkg.Meta[tlvLocalizedTitles]); err == nil {
		if name := cleanName(localized["en-us"]); name != "" {
			return name
		}
	}
	if name := cleanName(pkg.Meta[tlvTitleName]); name != "" {
		return name
	}
	if raw := pkg.Meta[tlvNacp]; len(raw) >= nacpLanguages*nacpEntry {
		for i := 0; i < nacpLanguages; i++ {
			if name := cleanName(cString(raw[i*nacpEntry:][:0x200])); name != "" {
				return name
			}
		}
	}
	if localized, err := parseLocalized(pkg.Meta[tlvLocalizedTitles]); err == nil {
		for _, tag := range languages {
			if name := cleanName(localized[tag]); name != "" {
				return name
			}
		}
	}
	return ""
}

// Publisher is the publisher a package gives, "" when it gives none.
func (pkg Package) Publisher() string {
	if publisher := text(pkg.Meta[tlvPublisher], 0x100); publisher != "" {
		return publisher
	}
	if raw := pkg.Meta[tlvNacp]; len(raw) >= nacpLanguages*nacpEntry {
		for i := 0; i < nacpLanguages; i++ {
			if publisher := text(cString(raw[i*nacpEntry+0x200:][:0x100]), 0x100); publisher != "" {
				return publisher
			}
		}
	}
	return ""
}

// DisplayVersion is the version as the game shows it (1.0.2), "" when the package does not say.
func (pkg Package) DisplayVersion() string {
	if version := text(pkg.Meta[tlvDisplayVersion], 0x0F); version != "" {
		return version
	}
	if raw := pkg.Meta[tlvNacp]; len(raw) >= nacpVersionAt+nacpVersionSize {
		return text(cString(raw[nacpVersionAt:][:nacpVersionSize]), 0x0F)
	}
	return ""
}

// Icon is the icon a package carries: the one of the default language, else American English,
// else the first of the others. Only JPEG and PNG images are returned.
func (pkg Package) Icon() []byte {
	image := func(data []byte) bool {
		return bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) || bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
	}
	if data := pkg.Meta[tlvIcon]; image(data) {
		return data
	}
	localized, err := parseLocalized(pkg.Meta[tlvLocalizedIcons])
	if err != nil {
		return nil
	}
	if data := localized["en-us"]; image(data) {
		return data
	}
	for _, tag := range languages {
		if data := localized[tag]; image(data) {
			return data
		}
	}
	return nil
}

// Icon is the icon the NUP carries for its game: the one of the game, else the one of an update,
// else the one of any package; nil when it carries none (the packer of the format writes none
// unless it is given one).
func (c *Container) Icon() []byte {
	for _, kind := range []byte{KindBase, KindUpdate, KindAddon} {
		for _, pkg := range c.Packages {
			if pkg.Kind == kind {
				if icon := pkg.Icon(); icon != nil {
					return icon
				}
			}
		}
	}
	return nil
}

// parseLocalized reads the records of names or icons by language: a length, a BCP-47 tag, then
// a length and the data.
func parseLocalized(payload []byte) (map[string][]byte, error) {
	result := map[string][]byte{}
	for pos := 0; pos < len(payload); {
		tagSize := int(payload[pos])
		pos++
		if tagSize == 0 || tagSize > len(payload)-pos-4 {
			return nil, errors.New("a localized record is malformed")
		}
		tag := strings.ToLower(string(payload[pos : pos+tagSize]))
		pos += tagSize
		size := int(binary.LittleEndian.Uint32(payload[pos:]))
		pos += 4
		if size < 0 || size > len(payload)-pos {
			return nil, errors.New("a localized record runs past the end of its block")
		}
		if _, seen := result[tag]; !seen {
			result[tag] = payload[pos : pos+size]
		}
		pos += size
	}
	return result, nil
}

// cString is the text of a field of a NACP, up to its first zero byte.
func cString(field []byte) []byte {
	if end := bytes.IndexByte(field, 0); end >= 0 {
		return field[:end]
	}
	return field
}

// text is a name, a publisher or a version stored in a NUP, cut to the length a NACP allows, as
// text without control characters.
func text(raw []byte, limit int) string {
	value := strings.ToValidUTF8(string(raw), "")
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

// nameTags are the title ID and the version some packers leave in the name they store, as a
// file name has them: "Game [0400245021D2C000][v131072]"
var nameTags = regexp.MustCompile(`\s*\[(?:[0-9A-Fa-f]{16}|[vV]\d+)\]`)

// cleanName is a name stored in a NUP as a name: as text, without the tags of a file name.
func cleanName(raw []byte) string {
	return strings.TrimSpace(nameTags.ReplaceAllString(text(raw, 0x200), ""))
}
