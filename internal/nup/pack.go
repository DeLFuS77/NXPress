package nup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

// The packing of a dump folder into a NUP, as the packer of the format does it
// (github.com/a55uka/nup, tools/nup): the game in base, each update in updates/<version>, each
// DLC in dlc/<title ID>/<index>, or the new layout of OunceCake, where each of them holds
// program/<index> with its exefs, romfs, control and the rest. Each file is stored compressed
// with zstd on its own, or as it is when zstd does not make it smaller, and files with the same
// data share one copy of it. The listings an FTP client leaves (.listing) are left out.

const (
	// a file up to this size is read whole and compressed by a worker; a bigger one is streamed
	// into the NUP by the writer
	wholeLimit = 8 << 20
	// the start of a big file compressed first: data it does not make smaller than this share
	// of itself is stored as it is, without compressing the whole file for nothing
	sampleSize  = 4 << 20
	sampleRatio = 0.97
	artifact    = ".listing"
)

// Source is a package found in a dump folder: its kind, its folder, and what it is.
type Source struct {
	Kind    byte
	Flags   byte
	Dir     string
	TitleID uint64
	Version uint32
	// main.npdm and the build ID of main, from its ExeFS
	Npdm    []byte
	BuildID []byte
	// its control data (control.nacp and the icons), from the new layout of OunceCake
	Control string
}

// IsDump tells whether a folder is a dump a NUP is made of: the exefs or romfs of a package, in
// it or in base, updates/<version> or dlc/<title ID>, as the packer of the format and OunceCake
// lay them out. A folder of a library that only has a folder named updates is not one.
func IsDump(dir string) bool {
	if isContent(dir) || isContent(filepath.Join(dir, "base")) {
		return true
	}
	for _, version := range sortedDirs(filepath.Join(dir, "updates")) {
		if _, err := strconv.ParseUint(version, 10, 32); err == nil && isContent(filepath.Join(dir, "updates", version)) {
			return true
		}
	}
	for _, id := range sortedDirs(filepath.Join(dir, "dlc")) {
		if _, err := strconv.ParseUint(id, 16, 64); err != nil || len(id) != 16 {
			continue
		}
		for _, entry := range sortedDirs(filepath.Join(dir, "dlc", id)) {
			if isContent(filepath.Join(dir, "dlc", id, entry)) || isContent(filepath.Join(dir, "dlc", id, entry, "base")) {
				return true
			}
		}
	}
	return false
}

// isContent tells whether a folder is the tree of a package: with its exefs or romfs, or with
// them in program/<index>.
func isContent(dir string) bool {
	for _, folder := range []string{dir, filepath.Join(dir, "program", "0")} {
		if isDir(filepath.Join(folder, "exefs")) || isDir(filepath.Join(folder, "romfs")) {
			return true
		}
	}
	return false
}

// Discover finds the packages of a dump folder, in the order a NUP keeps them: the game, its
// updates from the oldest, then its DLC. The notes tell what is left out.
func Discover(root string) ([]Source, []string, error) {
	var (
		result []Source
		notes  []string
	)
	base := filepath.Join(root, "base")
	updates := filepath.Join(root, "updates")
	dlcs := filepath.Join(root, "dlc")
	hasBase := isDir(base)
	switch {
	case hasBase || isDir(updates) || isDir(dlcs):
		gameID := uint64(0)
		if hasBase {
			if dir, err := contentDir(base); err != nil {
				return nil, nil, err
			} else if dir != "" {
				game := newSource(KindBase, dir)
				gameID = game.TitleID
				result = append(result, game)
			}
		}
		standalone := byte(0)
		if len(result) == 0 {
			standalone = 1
		}
		for _, entry := range sortedDirs(updates) {
			version, err := strconv.ParseUint(entry, 10, 32)
			if err != nil {
				notes = append(notes, "updates/"+entry+" left out: its name is not a version")
				continue
			}
			dir, err := contentDir(filepath.Join(updates, entry))
			if err != nil {
				return nil, nil, err
			}
			if dir == "" {
				continue
			}
			update := newSource(KindUpdate, dir)
			update.Version, update.Flags = uint32(version), standalone
			if update.TitleID == 0 {
				update.TitleID = gameID
			}
			result = append(result, update)
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].Kind < result[j].Kind || result[i].Kind == result[j].Kind && result[i].Version < result[j].Version
		})
		for _, id := range sortedDirs(dlcs) {
			titleID, err := strconv.ParseUint(id, 16, 64)
			if err != nil || len(id) != 16 {
				notes = append(notes, "dlc/"+id+" left out: its name is not a title ID")
				continue
			}
			for _, entry := range sortedDirs(filepath.Join(dlcs, id)) {
				folder := filepath.Join(dlcs, id, entry)
				// the new layout: dlc/<ID>/<version>/base, and its updates in updates/<version>
				if isDir(filepath.Join(folder, "base")) || isDir(filepath.Join(folder, "updates")) {
					version, err := strconv.ParseUint(entry, 10, 32)
					if err != nil {
						notes = append(notes, "dlc/"+id+"/"+entry+" left out: its name is not a version")
						continue
					}
					if isDir(filepath.Join(folder, "updates")) {
						notes = append(notes, "dlc/"+id+"/"+entry+"/updates left out: NUP version 1 holds no updates of DLC")
					}
					dir, err := contentDir(filepath.Join(folder, "base"))
					if err != nil {
						return nil, nil, err
					}
					if dir != "" {
						dlc := newSource(KindAddon, dir)
						dlc.TitleID, dlc.Version = titleID, uint32(version)
						result = append(result, dlc)
					}
					continue
				}
				dir, err := contentDir(folder)
				if err != nil {
					return nil, nil, err
				}
				if dir != "" {
					dlc := newSource(KindAddon, dir)
					dlc.TitleID = titleID
					result = append(result, dlc)
				}
			}
		}
	default:
		dir, err := contentDir(root)
		if err != nil {
			return nil, nil, err
		}
		if dir != "" {
			result = append(result, newSource(KindBase, dir))
		}
	}
	if len(result) == 0 {
		return nil, notes, errors.New("the folder holds no game, update or DLC to pack: no exefs or romfs with files in it")
	}
	// a game or an update without its main.npdm takes the title ID the name of its folder tells
	// ("Game [0100000000010000]"), as a NUP unpacked by NXPress is named
	if found := folderID.FindStringSubmatch(filepath.Base(filepath.Clean(root))); found != nil {
		id, _ := strconv.ParseUint(found[1], 16, 64)
		for i := range result {
			if result[i].TitleID == 0 && result[i].Kind != KindAddon {
				result[i].TitleID = id &^ 0xFFF
			}
		}
	}
	return result, notes, nil
}

// folderID is the title ID in the name of a dump folder.
var folderID = regexp.MustCompile(`\[([0-9A-Fa-f]{16})\]`)

// contentDir is the folder of the tree of a package: the folder itself in the layout of the
// packer of the format, program/<index> in the new layout of OunceCake; "" when it holds no
// file. A title of several programs is not held by NUP version 1.
func contentDir(dir string) (string, error) {
	programs := []string{}
	for _, entry := range sortedDirs(filepath.Join(dir, "program")) {
		if _, err := strconv.Atoi(entry); err == nil {
			programs = append(programs, entry)
		}
	}
	switch {
	case len(programs) > 1:
		return "", fmt.Errorf("it holds %d programs (%s): NUP version 1 holds one", len(programs), strings.Join(programs, ", "))
	case len(programs) == 1:
		dir = filepath.Join(dir, "program", programs[0])
	}
	if !hasFiles(dir) {
		return "", nil
	}
	return dir, nil
}

// newSource is a package of a folder, with its ID and build ID read from its ExeFS.
func newSource(kind byte, dir string) Source {
	source := Source{Kind: kind, Dir: dir}
	exefs := findDir(dir, "exefs")
	if exefs != "" {
		if npdm, err := os.ReadFile(filepath.Join(exefs, "main.npdm")); err == nil {
			source.Npdm = npdm
			source.TitleID = npdmProgramID(npdm)
		}
		if file, err := os.Open(filepath.Join(exefs, "main")); err == nil {
			head := make([]byte, 0x60)
			if _, err := io.ReadFull(file, head); err == nil && (string(head[:4]) == "NSO0" || string(head[0x10:0x14]) == "NRO0") {
				source.BuildID = head[0x40:0x60]
			}
			file.Close()
		}
	}
	if control := findDir(dir, "control"); control != "" {
		source.Control = control
	}
	return source
}

// findDir is the folder of a name in the tree of a package: at its root, or in program/ when
// the program and the data of a title share their folder.
func findDir(dir string, name string) string {
	for _, candidate := range []string{filepath.Join(dir, name), filepath.Join(dir, "program", name)} {
		if isDir(candidate) {
			return candidate
		}
	}
	return ""
}

// npdmProgramID is the program ID of an NPDM, from its ACI0.
func npdmProgramID(npdm []byte) uint64 {
	le := binary.LittleEndian
	if len(npdm) >= 0x78 && string(npdm[:4]) == "META" {
		if at := int(le.Uint32(npdm[0x70:])); at >= 0 && at+0x18 <= len(npdm) && string(npdm[at:at+4]) == "ACI0" {
			return le.Uint64(npdm[at+0x10:])
		}
	}
	if at := bytes.Index(npdm, []byte("ACI0")); at >= 0 && at+0x18 <= len(npdm) {
		return le.Uint64(npdm[at+0x10:])
	}
	return 0
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// sortedDirs are the names of the folders in a folder, in order.
func sortedDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// hasFiles tells whether a folder holds a file, in it or below, that is not an artifact.
func hasFiles(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() != artifact {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// packNode is a folder or a file of the tree being packed.
type packNode struct {
	name     string
	path     string
	size     int64
	children []*packNode
	dir      bool
	index    int
}

// packTree is the tree of every package, numbered as the table of contents keeps it: the folders
// in the order they are met, each with its children together, the files in the order their
// folders name them.
type packTree struct {
	dirs     []*packNode
	files    []*packNode
	children []uint32
	firsts   []uint32
	pool     bytes.Buffer
	names    map[string]uint32
}

// walk reads the tree of a folder, its folders read at once on the threads free (lanes), each
// node in the order of its name.
func walk(path string, name string, lanes chan struct{}) (*packNode, error) {
	node := &packNode{name: name, path: path, dir: true}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	// os.ReadDir sorts by name, which is the byte order of their UTF-8 the format asks for
	children := make([]*packNode, len(entries))
	var (
		wait  sync.WaitGroup
		mutex sync.Mutex
		first error
	)
	failed := func(err error) {
		mutex.Lock()
		if first == nil {
			first = err
		}
		mutex.Unlock()
	}
	for i, entry := range entries {
		full := filepath.Join(path, entry.Name())
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			failed(fmt.Errorf("%s is a link: links are not packed", full))
		case entry.IsDir():
			select {
			case lanes <- struct{}{}:
				wait.Add(1)
				go func() {
					defer wait.Done()
					defer func() { <-lanes }()
					child, err := walk(full, entry.Name(), lanes)
					if err != nil {
						failed(err)
					}
					children[i] = child
				}()
			default:
				child, err := walk(full, entry.Name(), lanes)
				if err != nil {
					failed(err)
				}
				children[i] = child
			}
		case entry.Name() == artifact:
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				failed(err)
				continue
			}
			children[i] = &packNode{name: entry.Name(), path: full, size: info.Size()}
		default:
			failed(fmt.Errorf("%s is not a file or a folder", full))
		}
	}
	wait.Wait()
	if first != nil {
		return nil, first
	}
	for _, child := range children {
		if child != nil {
			node.children = append(node.children, child)
		}
	}
	return node, nil
}

// assign numbers the folders and the files from the roots of the packages.
func (t *packTree) assign(root *packNode) {
	var visit func(dir *packNode)
	visit = func(dir *packNode) {
		dir.index = len(t.dirs)
		t.dirs = append(t.dirs, dir)
		first := len(t.children)
		t.firsts = append(t.firsts, uint32(first))
		t.children = append(t.children, make([]uint32, len(dir.children))...)
		for i, child := range dir.children {
			if !child.dir {
				child.index = len(t.files)
				t.files = append(t.files, child)
				t.children[first+i] = fileTag + uint32(child.index)
			}
		}
		for i, child := range dir.children {
			if child.dir {
				visit(child)
				t.children[first+i] = uint32(child.index)
			}
		}
	}
	visit(root)
}

func (t *packTree) intern(name string) (uint32, uint32) {
	if at, ok := t.names[name]; ok {
		return at, uint32(len(name))
	}
	at := uint32(t.pool.Len())
	t.names[name] = at
	t.pool.WriteString(name)
	return at, uint32(len(name))
}

// PackOptions are how a NUP is packed.
type PackOptions struct {
	// the level of zstd, 3 as the packer of the format; 0 stores every file as it is
	Level int
	// the files read and compressed at once, and the ones read at once from the drive of the
	// folder: one for a hard disk, more for an SSD
	Workers int
	Readers int
	// the name and the publisher of the game, when its folder has no control data to tell them
	Name      string
	Publisher string
}

// PackResult is what a NUP packed holds.
type PackResult struct {
	Packages []Source
	Notes    []string
	Files    int
	// the files uncompressed, and the NUP written
	Size     int64
	FileSize int64
	// the files whose data another file shares, and the ones stored with zstd
	Shared int
	Zstd   int
}

// fileOut is where the data of a file went in the NUP.
type fileOut struct {
	offset, stored uint64
	size           uint64
	hash           [32]byte
	codec          byte
}

// Pack writes the NUP of a dump folder to out, a file that must not be there yet. progress is
// told the bytes of files packed of the ones to pack. A file of the folder that changes while it
// is packed fails the work; out is then left to the caller to remove.
func Pack(ctx context.Context, src string, out string, o PackOptions, progress func(done int64, total int64)) (PackResult, error) {
	sources, notes, err := Discover(src)
	if err != nil {
		return PackResult{}, err
	}
	result := PackResult{Packages: sources, Notes: notes}
	tree := &packTree{names: map[string]uint32{}}
	roots := make([]*packNode, len(sources))
	for i, source := range sources {
		if roots[i], err = walk(source.Dir, "", make(chan struct{}, max(1, o.Workers))); err != nil {
			return result, err
		}
		tree.assign(roots[i])
	}
	if len(tree.files) > maxTocSize/fileSize {
		return result, fmt.Errorf("the folder holds %d files, more than a NUP can", len(tree.files))
	}
	for _, dir := range tree.dirs {
		tree.intern(dir.name)
	}
	for _, file := range tree.files {
		tree.intern(file.name)
	}
	dirCount, fileCount := len(tree.dirs), len(tree.files)
	records := tocHeadSize + len(sources)*packageSize + dirCount*dirSize + fileCount*fileSize
	poolOffset := records + len(tree.children)*4
	tocSize := poolOffset + tree.pool.Len()
	if tocSize > maxTocSize {
		return result, errors.New("the table of contents of the folder would be too big for a NUP")
	}
	blobOffset := (headerSize + tocSize + 15) &^ 15

	file, err := os.OpenFile(out, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return result, err
	}
	defer file.Close()
	writer := &blobWriter{file: file, buffer: bufio.NewWriterSize(file, 4<<20)}
	if err := writer.zeros(int64(blobOffset)); err != nil {
		return result, err
	}

	total := int64(0)
	for _, node := range tree.files {
		total += node.size
	}
	result.Files, result.Size = fileCount, total
	outs, err := writeExtents(ctx, writer, out, tree.files, o, total, progress)
	if err != nil {
		return result, err
	}
	for _, f := range outs {
		if f.codec == CodecZstd {
			result.Zstd++
		}
	}
	seen := map[uint64]bool{}
	for _, f := range outs {
		if f.size > 0 {
			if seen[f.offset] {
				result.Shared++
			}
			seen[f.offset] = true
		}
	}

	// the metadata of each package, after the data
	metas := make([][2]uint64, len(sources))
	label := filepath.Base(filepath.Clean(src))
	for i, source := range sources {
		block := metaBlock(source, label, i == appPackage(sources), o)
		if len(block) == 0 {
			continue
		}
		if err := writer.align(); err != nil {
			return result, err
		}
		metas[i] = [2]uint64{uint64(writer.at), uint64(len(block))}
		if err := writer.write(block); err != nil {
			return result, err
		}
	}
	end := writer.at
	if err := writer.buffer.Flush(); err != nil {
		return result, err
	}

	le := binary.LittleEndian
	toc := make([]byte, tocSize)
	copy(toc, tocMagic)
	le.PutUint32(toc[0x04:], uint32(len(sources)))
	le.PutUint64(toc[0x08:], uint64(dirCount))
	le.PutUint64(toc[0x10:], uint64(fileCount))
	le.PutUint64(toc[0x18:], uint64(poolOffset))
	le.PutUint64(toc[0x20:], uint64(tree.pool.Len()))
	for i, source := range sources {
		record := toc[tocHeadSize+i*packageSize:]
		le.PutUint64(record[0x00:], source.TitleID)
		le.PutUint32(record[0x08:], uint32(roots[i].index))
		le.PutUint32(record[0x0C:], dirIndex(roots[i], "exefs"))
		le.PutUint32(record[0x10:], dirIndex(roots[i], "romfs"))
		le.PutUint32(record[0x14:], source.Version)
		record[0x18] = source.Kind
		record[0x19] = source.Flags
		le.PutUint64(record[0x20:], metas[i][0])
		le.PutUint32(record[0x28:], uint32(metas[i][1]))
	}
	position := tocHeadSize + len(sources)*packageSize
	for i, dir := range tree.dirs {
		at, length := tree.intern(dir.name)
		le.PutUint32(toc[position:], at)
		le.PutUint32(toc[position+4:], length)
		le.PutUint32(toc[position+8:], tree.firsts[i])
		le.PutUint32(toc[position+12:], uint32(len(dir.children)))
		position += dirSize
	}
	for i, node := range tree.files {
		at, length := tree.intern(node.name)
		f := outs[i]
		le.PutUint32(toc[position:], at)
		le.PutUint32(toc[position+4:], length)
		le.PutUint64(toc[position+0x08:], f.size)
		le.PutUint64(toc[position+0x10:], f.offset)
		le.PutUint64(toc[position+0x18:], f.stored)
		copy(toc[position+0x20:], f.hash[:])
		toc[position+0x40] = f.codec
		position += fileSize
	}
	for _, child := range tree.children {
		if child >= fileTag {
			child = uint32(dirCount) + child - fileTag
		}
		le.PutUint32(toc[position:], child)
		position += 4
	}
	copy(toc[poolOffset:], tree.pool.Bytes())

	header := make([]byte, headerSize)
	copy(header, magic)
	le.PutUint16(header[0x04:], headerSize)
	le.PutUint16(header[0x06:], 1)
	le.PutUint64(header[0x08:], headerSize)
	le.PutUint64(header[0x10:], uint64(tocSize))
	sum := sha256.Sum256(toc)
	copy(header[0x18:], sum[:])
	le.PutUint64(header[0x38:], uint64(blobOffset))
	le.PutUint64(header[0x40:], uint64(end))
	le.PutUint32(header[0x48:], uint32(len(sources)))
	uuid := sha256.New()
	for _, source := range sources {
		var id [13]byte
		le.PutUint64(id[0:], source.TitleID)
		le.PutUint32(id[8:], source.Version)
		id[12] = source.Kind
		uuid.Write(id[:])
	}
	copy(header[0x50:0x60], uuid.Sum(nil))
	if _, err := file.WriteAt(append(header, toc...), 0); err != nil {
		return result, err
	}
	if err := file.Truncate(end); err != nil {
		return result, err
	}
	if err := file.Sync(); err != nil {
		return result, err
	}
	result.FileSize = end
	return result, nil
}

// dirIndex is the folder of a name at the root of a package, or in program/, NoDir when it has
// none.
func dirIndex(root *packNode, name string) uint32 {
	for _, child := range root.children {
		if child.dir && child.name == name {
			return uint32(child.index)
		}
	}
	for _, child := range root.children {
		if child.dir && child.name == "program" {
			for _, inner := range child.children {
				if inner.dir && inner.name == name {
					return uint32(inner.index)
				}
			}
		}
	}
	return NoDir
}

// appPackage is the package that carries the name, the publisher and the icon of the game: the
// game, else the first package, as the format asks.
func appPackage(sources []Source) int {
	for i, source := range sources {
		if source.Kind == KindBase {
			return i
		}
	}
	return 0
}

// metaBlock is the metadata of a package: its NPDM, its build ID and the dump it comes from; for
// the game also its names, publisher, version and icon, from its control data or as given.
func metaBlock(source Source, label string, app bool, o PackOptions) []byte {
	records := map[uint16][]byte{}
	if len(source.Npdm) > 0 {
		records[0x0001] = source.Npdm
	}
	if len(source.BuildID) > 0 {
		records[0x0007] = source.BuildID
	}
	records[0x0008] = []byte(label)
	if app {
		for kind, value := range controlRecords(source.Control) {
			records[kind] = value
		}
		if o.Name != "" {
			records[tlvTitleName] = []byte(o.Name)
		}
		if o.Publisher != "" {
			records[tlvPublisher] = []byte(o.Publisher)
		}
	}
	kinds := []int{}
	for kind := range records {
		kinds = append(kinds, int(kind))
	}
	sort.Ints(kinds)
	var block bytes.Buffer
	for _, kind := range kinds {
		value := records[uint16(kind)]
		var head [tlvHeadSize]byte
		binary.LittleEndian.PutUint16(head[0:], uint16(kind))
		binary.LittleEndian.PutUint32(head[4:], uint32(len(value)))
		block.Write(head[:])
		block.Write(value)
	}
	return block.Bytes()
}

// nacpLanguageNames are the names of the languages of a NACP, as the icons of a control
// filesystem are named: icon_AmericanEnglish.dat
var nacpLanguageNames = [nacpLanguages]string{"AmericanEnglish", "BritishEnglish", "Japanese", "French", "German",
	"LatinAmericanSpanish", "Spanish", "Italian", "Dutch", "CanadianFrench", "Portuguese", "Russian", "Korean",
	"TraditionalChinese", "SimplifiedChinese", "BrazilianPortuguese"}

// controlRecords are the metadata a control folder gives: the NACP as it is, the name and the
// publisher of the game in each language, its version, and its icons.
func controlRecords(dir string) map[uint16][]byte {
	result := map[uint16][]byte{}
	if dir == "" {
		return result
	}
	nacp, err := os.ReadFile(filepath.Join(dir, "control.nacp"))
	if err == nil && len(nacp) >= nacpVersionAt+nacpVersionSize {
		result[tlvNacp] = nacp
		var titles bytes.Buffer
		for i, tag := range languages {
			name := text(cString(nacp[i*nacpEntry:][:0x200]), 0x200)
			if name == "" {
				continue
			}
			titles.Write(localized(tag, []byte(name)))
			if _, ok := result[tlvTitleName]; !ok || i == 0 {
				result[tlvTitleName] = []byte(name)
				if publisher := text(cString(nacp[i*nacpEntry+0x200:][:0x100]), 0x100); publisher != "" {
					result[tlvPublisher] = []byte(publisher)
				}
			}
		}
		if titles.Len() > 0 {
			result[tlvLocalizedTitles] = titles.Bytes()
		}
		if version := text(cString(nacp[nacpVersionAt:][:nacpVersionSize]), 0x0F); version != "" {
			result[tlvDisplayVersion] = []byte(version)
		}
	}
	var icons bytes.Buffer
	for i, name := range nacpLanguageNames {
		icon, err := os.ReadFile(filepath.Join(dir, "icon_"+name+".dat"))
		if err != nil || len(icon) == 0 || len(icon) > maxMetaSize/32 {
			continue
		}
		icons.Write(localized(languages[i], icon))
		if _, ok := result[tlvIcon]; !ok || i == 0 {
			result[tlvIcon] = icon
		}
	}
	if icons.Len() > 0 {
		result[tlvLocalizedIcons] = icons.Bytes()
	}
	return result
}

// ControlName is the name of the game a control folder gives: American English, else the first
// language that has one; "" without one.
func ControlName(dir string) string {
	if dir == "" {
		return ""
	}
	return cleanName(controlRecords(dir)[tlvTitleName])
}

// localized is one record of names or icons by language.
func localized(tag string, data []byte) []byte {
	out := []byte{byte(len(tag))}
	out = append(out, tag...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	return append(out, data...)
}

// blobWriter writes the NUP in order, can leave a place for a file written by another worker, and
// can go back to a place it wrote, to write over a piece of data another file already has.
type blobWriter struct {
	file   *os.File
	buffer *bufio.Writer
	at     int64
}

func (w *blobWriter) write(data []byte) error {
	n, err := w.buffer.Write(data)
	w.at += int64(n)
	return err
}

func (w *blobWriter) Write(data []byte) (int, error) {
	err := w.write(data)
	return len(data), err
}

func (w *blobWriter) zeros(n int64) error {
	for n > 0 {
		chunk := min(n, 64<<10)
		if err := w.write(make([]byte, chunk)); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}

func (w *blobWriter) align() error {
	return w.zeros((16 - w.at%16) % 16)
}

// back goes back to a place written, to write there again.
func (w *blobWriter) back(at int64) error {
	return w.seek(at)
}

// leave leaves size bytes for a file another worker writes, and goes on after them.
func (w *blobWriter) leave(size int64) error {
	return w.seek(w.at + size)
}

func (w *blobWriter) seek(at int64) error {
	if err := w.buffer.Flush(); err != nil {
		return err
	}
	if _, err := w.file.Seek(at, io.SeekStart); err != nil {
		return err
	}
	w.buffer.Reset(w.file)
	w.at = at
	return nil
}

// prepared is a file read by a worker: its hash, and its data as it goes into the NUP; a big file
// has its data left to the writer or a copier, with its hash when another file has its size.
type prepared struct {
	index  int
	hash   [32]byte
	hashed bool
	data   []byte
	codec  byte
	big    bool
	// a big file zstd makes smaller, by its start
	compress bool
	err      error
}

// copyJob is a big file a copier writes into the place the writer left for it.
type copyJob struct {
	index  int
	offset int64
	hash   [32]byte
	hashed bool
}

// writeExtents writes the data of every file into the NUP, each piece once. The workers read,
// hash and compress the small files and look at the big ones; the writer writes what they give
// in the order they give it; a big file zstd does not make smaller goes into a place the writer
// leaves for it, written by a copier while the writer goes on, and a big file zstd makes smaller
// is streamed by the writer, with zstd on every worker. No more files are read at once than the
// drive of the folder reads best (o.Readers). A big file whose size another file has is hashed
// first, so its data is written once.
func writeExtents(ctx context.Context, w *blobWriter, out string, files []*packNode, o PackOptions, total int64,
	progress func(done int64, total int64)) ([]fileOut, error) {
	outs := make([]fileOut, len(files))
	workers := max(1, o.Workers)
	slots := make(chan struct{}, max(1, o.Readers))
	level := zstd.EncoderLevelFromZstd(max(1, o.Level))
	sizes := map[int64]int{}
	bigs := 0
	for _, node := range files {
		if node.size > wholeLimit {
			sizes[node.size]++
			bigs++
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		failure atomic.Value
		done    atomic.Int64
		told    sync.Mutex
	)
	fail := func(err error) {
		if failure.CompareAndSwap(nil, err) {
			cancel()
		}
	}
	tell := func(n int64) {
		value := done.Add(n)
		if progress != nil && told.TryLock() {
			progress(value, total)
			told.Unlock()
		}
	}
	if progress != nil {
		progress(0, total)
	}

	jobs := make(chan int)
	results := make(chan prepared, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var encoder *zstd.Encoder
			if o.Level > 0 {
				encoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(level), zstd.WithEncoderConcurrency(1))
				defer encoder.Close()
			}
			for index := range jobs {
				p := prepareFile(ctx, index, files[index], encoder, sizes[files[index].size] > 1, slots)
				select {
				case results <- p:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range files {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wait.Wait()
		close(results)
	}()

	// the copiers, each with a handle of its own on the NUP
	copies := make(chan copyJob, bigs+1)
	var copiers sync.WaitGroup
	for range workers {
		copiers.Add(1)
		go func() {
			defer copiers.Done()
			target, err := os.OpenFile(out, os.O_WRONLY, 0)
			if err != nil {
				fail(err)
				for range copies {
				}
				return
			}
			defer target.Close()
			buffer := make([]byte, 1<<20)
			for job := range copies {
				if ctx.Err() != nil {
					continue
				}
				node := files[job.index]
				slots <- struct{}{}
				hash, err := copyAt(ctx, target, job.offset, node, buffer, tell)
				<-slots
				switch {
				case err != nil:
					fail(err)
				case job.hashed && hash != job.hash:
					fail(fmt.Errorf("%s changed while it was packed", node.path))
				case !job.hashed:
					// only a file whose size no other file has: the writer never reads it
					outs[job.index].hash = hash
				}
			}
		}()
	}
	finishCopies := func() {
		close(copies)
		copiers.Wait()
	}

	var streamer *zstd.Encoder
	if o.Level > 0 {
		streamer, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(level), zstd.WithEncoderConcurrency(workers))
		defer streamer.Close()
	}
	type key struct {
		hash [32]byte
		size uint64
	}
	seen := map[key]int{}
	for p := range results {
		if ctx.Err() != nil {
			break
		}
		if p.err != nil {
			fail(p.err)
			break
		}
		node := files[p.index]
		size := uint64(node.size)
		if size == 0 {
			outs[p.index] = fileOut{hash: sha256.Sum256(nil)}
			continue
		}
		// the same data is kept once: a file whose data is there already is not written
		if p.hashed {
			if known, ok := seen[key{p.hash, size}]; ok {
				outs[p.index] = outs[known]
				outs[p.index].hash = p.hash
				tell(node.size)
				continue
			}
		}
		start := w.at
		if err := w.align(); err != nil {
			fail(err)
			break
		}
		at := w.at
		switch {
		case !p.big:
			if err := w.write(p.data); err != nil {
				fail(err)
				break
			}
			outs[p.index] = fileOut{offset: uint64(at), stored: uint64(len(p.data)), size: size, hash: p.hash, codec: p.codec}
			tell(node.size)
		case !p.compress:
			// a place for it, written by a copier while the writer goes on
			if err := w.leave(node.size); err != nil {
				fail(err)
				break
			}
			outs[p.index] = fileOut{offset: uint64(at), stored: size, size: size, hash: p.hash, codec: CodecRaw}
			copies <- copyJob{index: p.index, offset: at, hash: p.hash, hashed: p.hashed}
		default:
			slots <- struct{}{}
			out, err := streamFile(ctx, w, node, streamer, tell)
			<-slots
			if err == nil && p.hashed && out.hash != p.hash {
				err = fmt.Errorf("%s changed while it was packed", node.path)
			}
			if err != nil {
				fail(err)
				break
			}
			if known, ok := seen[key{out.hash, size}]; ok {
				if err := w.back(start); err != nil {
					fail(err)
					break
				}
				out = outs[known]
			}
			outs[p.index] = out
		}
		if ctx.Err() != nil {
			break
		}
		if p.hashed || !p.big {
			if _, ok := seen[key{outs[p.index].hash, size}]; !ok {
				seen[key{outs[p.index].hash, size}] = p.index
			}
		}
	}
	// what is left of the results is let go, so the workers end
	go func() {
		for range results {
		}
	}()
	finishCopies()
	if err, _ := failure.Load().(error); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return outs, nil
}

// prepareFile reads a small file whole, hashes it and compresses it, keeping it as it is when
// zstd does not make it smaller. Of a big file it tells whether zstd makes its start smaller, and
// hashes it when another file has its size (it may hold the same data). A file is read only
// while a slot of the drive is free.
func prepareFile(ctx context.Context, index int, node *packNode, encoder *zstd.Encoder, shared bool, slots chan struct{}) prepared {
	p := prepared{index: index}
	if node.size == 0 || ctx.Err() != nil {
		return p
	}
	slots <- struct{}{}
	defer func() { <-slots }()
	if node.size > wholeLimit {
		p.big = true
		file, err := os.Open(node.path)
		if err != nil {
			p.err = err
			return p
		}
		defer file.Close()
		if encoder != nil {
			sample := make([]byte, sampleSize)
			if _, err := io.ReadFull(file, sample); err != nil {
				p.err = fmt.Errorf("%s: %w", node.path, err)
				return p
			}
			p.compress = float64(len(encoder.EncodeAll(sample, nil))) < float64(len(sample))*sampleRatio
		}
		if shared {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				p.err = err
				return p
			}
			hash := sha256.New()
			n, err := io.CopyBuffer(hash, &ctxReader{ctx: ctx, r: file}, make([]byte, 1<<20))
			if err != nil {
				p.err = err
				return p
			}
			if n != node.size {
				p.err = fmt.Errorf("%s changed while it was packed", node.path)
				return p
			}
			copy(p.hash[:], hash.Sum(nil))
			p.hashed = true
		}
		return p
	}
	data, err := os.ReadFile(node.path)
	if err != nil {
		p.err = err
		return p
	}
	if int64(len(data)) != node.size {
		p.err = fmt.Errorf("%s changed while it was packed", node.path)
		return p
	}
	p.hash, p.hashed = sha256.Sum256(data), true
	p.data, p.codec = data, CodecRaw
	if encoder != nil {
		if compressed := encoder.EncodeAll(data, make([]byte, 0, len(data)/2)); len(compressed) < len(data) {
			p.data, p.codec = compressed, CodecZstd
		}
	}
	return p
}

// streamFile writes a big file zstd makes smaller into the NUP as it reads it, with zstd on
// every worker; when the whole of it was not made smaller, it is written again as it is.
func streamFile(ctx context.Context, w *blobWriter, node *packNode, streamer *zstd.Encoder, tell func(int64)) (fileOut, error) {
	at := w.at
	size := node.size
	streamer.ResetContentSize(w, size)
	read := int64(0)
	hash, n, err := copyFile(ctx, streamer, node, func(total int64) {
		tell(total - read)
		read = total
	})
	if err == nil {
		err = streamer.Close()
	}
	if err != nil {
		return fileOut{}, err
	}
	if n != size {
		return fileOut{}, fmt.Errorf("%s changed while it was packed", node.path)
	}
	if stored := w.at - at; stored < size {
		return fileOut{offset: uint64(at), stored: uint64(stored), size: uint64(size), hash: hash, codec: CodecZstd}, nil
	}
	if err := w.back(at); err != nil {
		return fileOut{}, err
	}
	again, n, err := copyFile(ctx, w, node, nil)
	if err != nil {
		return fileOut{}, err
	}
	if n != size || again != hash {
		return fileOut{}, fmt.Errorf("%s changed while it was packed", node.path)
	}
	return fileOut{offset: uint64(at), stored: uint64(size), size: uint64(size), hash: hash, codec: CodecRaw}, nil
}

// copyAt writes a big file into the place left for it in the NUP, hashing it as it goes, and
// tells the bytes read.
func copyAt(ctx context.Context, target *os.File, offset int64, node *packNode, buffer []byte, tell func(int64)) ([32]byte, error) {
	file, err := os.Open(node.path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	at, read := offset, int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			if read+int64(n) > node.size {
				return [32]byte{}, fmt.Errorf("%s changed while it was packed", node.path)
			}
			hash.Write(buffer[:n])
			if _, werr := target.WriteAt(buffer[:n], at); werr != nil {
				return [32]byte{}, werr
			}
			at += int64(n)
			read += int64(n)
			tell(int64(n))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return [32]byte{}, err
		}
	}
	if read != node.size {
		return [32]byte{}, fmt.Errorf("%s changed while it was packed", node.path)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}

// copyFile writes a file into to, hashing it as it goes, and gives its hash and its bytes; read
// is told the bytes read so far.
func copyFile(ctx context.Context, to io.Writer, node *packNode, read func(n int64)) ([32]byte, int64, error) {
	file, err := os.Open(node.path)
	if err != nil {
		return [32]byte{}, 0, err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.CopyBuffer(io.MultiWriter(to, hash), &ctxReader{ctx: ctx, r: file, read: read}, make([]byte, 1<<20))
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, n, err
}

// ctxReader stops reading when the work is cancelled, and tells the bytes read.
type ctxReader struct {
	ctx  context.Context
	r    io.Reader
	read func(n int64)
	n    int64
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	r.n += int64(n)
	if r.read != nil {
		r.read(r.n)
	}
	return n, err
}
