// Package nxpack writes .nx files (github.com/fab918/nxformat, header version 0.2): games, their
// updates and their DLC, decrypted, for emulators. Each version is a complete tree of files; the
// files are cut in blocks of 64 or 256 KiB, each stored once however many files and versions hold
// it, with zstd when that saves at least a quarter of it, else raw, and each checked by a
// CRC-32C for every 16 KiB and the SHA-256 of every file. The file is written beside its place
// and put there whole once read back.
package nxpack

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

// kinds of versions
const (
	Base             = 0
	Update           = 1
	Dlc              = 2
	StandaloneUpdate = 3
)

const (
	slot          = 4096
	headerSize    = 256
	catalogHeader = 128
	appSize       = 80
	programSize   = 64
	snapshotSize  = 120
	localizedSize = 24
	mediaHeader   = 32
	mediaRecord   = 64
	manifestHead  = 96
	dirSize       = 16
	entrySize     = 24
	fileSize      = 64
	blockSize     = 24
	checkSize     = 16384
	absent        = 0xffffffff
	smallChunk    = 16 // 64 KiB, for files whose first chunk compresses
	bigChunk      = 18 // 256 KiB
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// File is a file of a version: its path in the tree ("exefs/main", folders apart with "/"), and
// the file on disk it is read from.
type File struct {
	Path   string
	Source string
	Size   int64
}

// Snapshot is a version of a game, complete: the game, an update (with the files of the game it
// does not replace) or a DLC.
type Snapshot struct {
	Kind    byte
	Version uint32
	// the program of the game and its updates; 0 for a DLC
	Program uint64
	// the title ID of a DLC
	Addon   uint64
	Display string
	BuildID []byte
	Files   []File
	// its folders, the empty ones too ("exefs", "romfs/Data")
	Dirs []string
	// its control data: the NACP, the icon, the name and publisher, the names by language tag
	Nacp      []byte
	Icon      []byte
	Title     string
	Publisher string
	Names     map[string]string
}

// Game is a game of the file, with all its versions.
type Game struct {
	ID       uint64
	Platform byte // 0 unknown, 1 Switch, 2 Switch 2
	Versions []Snapshot
}

// Options are how a file is written.
type Options struct {
	// the level of zstd, 1 to 13 (3 by default)
	Level   int
	Workers int
}

// Result is what a file written holds.
type Result struct {
	Files    int
	Blocks   int
	Size     int64 // the bytes of the files of all the versions
	FileSize int64
}

// zstd keeps a frame only when it saves at least a quarter of the block
const keepRatio = 0.75

type block struct {
	offset     uint64
	stored     uint32
	logical    uint32
	codec      byte
	firstCheck uint32
}

type fileRecord struct {
	size   uint64
	first  uint32
	count  uint32
	log2   byte
	sha    [32]byte
	key    string // path and content, to share the record between versions of a game
	blocks []uint32
}

type writer struct {
	ctx      context.Context
	out      *os.File
	buffered *bufio.Writer
	offset   uint64
	workers  int
	blocks   []block
	checks   []uint32
	// the blocks by the SHA-256 of their bytes, to store each once
	known    map[[32]byte][]uint32
	files    []fileRecord
	byKey    map[string]int
	refs     []uint32
	decoder  *zstd.Decoder
	encoder  *zstd.Encoder
	result   Result
	done     int64
	total    int64
	progress func(done int64, total int64)
}

// Write writes a .nx file of the games given at out, which must not be there.
func Write(ctx context.Context, out string, games []Game, o Options, progress func(done int64, total int64)) (Result, error) {
	if len(games) == 0 {
		return Result{}, errors.New("nothing to write")
	}
	if err := check(games); err != nil {
		return Result{}, err
	}
	if _, err := os.Lstat(out); err == nil {
		return Result{}, fmt.Errorf("%s is there already", filepath.Base(out))
	}
	level := o.Level
	if level == 0 {
		level = 3
	}
	if level < 1 || level > 13 {
		return Result{}, errors.New("the level of zstd goes from 1 to 13")
	}
	workers := o.Workers
	if workers < 1 {
		workers = runtime.NumCPU()
	}
	temporary := out + ".part"
	file, err := os.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Result{}, err
	}
	finished := false
	defer func() {
		if !finished {
			file.Close()
			os.Remove(temporary)
		}
	}()
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return Result{}, err
	}
	defer decoder.Close()
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)), zstd.WithEncoderConcurrency(workers),
		zstd.WithSingleSegment(true), zstd.WithEncoderCRC(false))
	if err != nil {
		return Result{}, err
	}
	defer encoder.Close()
	w := &writer{ctx: ctx, out: file, buffered: bufio.NewWriterSize(file, 4<<20),
		workers: workers, encoder: encoder, known: map[[32]byte][]uint32{}, byKey: map[string]int{}, decoder: decoder, progress: progress}
	for _, game := range games {
		for _, v := range game.Versions {
			for _, f := range v.Files {
				w.total += f.Size
			}
		}
	}
	if err := w.zeros(2 * slot); err != nil {
		return Result{}, err
	}
	// the files of every version, in the order of their trees; their records by version
	trees := make([][][]int, len(games))
	for g, game := range games {
		trees[g] = make([][]int, len(game.Versions))
		for s, v := range game.Versions {
			files := append([]File(nil), v.Files...)
			sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
			for _, f := range files {
				index, err := w.addFile(fmt.Sprintf("%d|%s", g, f.Path), f)
				if err != nil {
					return Result{}, err
				}
				trees[g][s] = append(trees[g][s], index)
			}
		}
	}
	if err := w.buffered.Flush(); err != nil {
		return Result{}, err
	}
	dataEnd := w.offset
	if len(w.blocks) == 0 {
		dataEnd = 2 * slot
	}
	tail, layout, err := buildTail(games, trees, w, dataEnd)
	if err != nil {
		return Result{}, err
	}
	if _, err := file.WriteAt(tail, int64(align(dataEnd, 16))); err != nil {
		return Result{}, err
	}
	header := layout.header()
	if _, err := file.WriteAt(header, 0); err != nil {
		return Result{}, err
	}
	if _, err := file.WriteAt(header, slot); err != nil {
		return Result{}, err
	}
	if err := file.Truncate(int64(layout.archiveSize)); err != nil {
		return Result{}, err
	}
	if err := file.Sync(); err != nil {
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}
	if err := os.Rename(temporary, out); err != nil {
		os.Remove(temporary)
		return Result{}, err
	}
	finished = true
	w.result.Blocks = len(w.blocks)
	w.result.FileSize = int64(layout.archiveSize)
	return w.result, nil
}

// check refuses what the format cannot hold before anything is written.
func check(games []Game) error {
	type key struct {
		platform byte
		id       uint64
	}
	seen := map[key]bool{}
	for _, game := range games {
		if game.ID == 0 || game.Platform > 2 {
			return errors.New("a game without an ID cannot be written")
		}
		k := key{game.Platform, game.ID}
		if seen[k] {
			return fmt.Errorf("the game %016x is given twice", game.ID)
		}
		seen[k] = true
		if len(game.Versions) == 0 {
			return fmt.Errorf("the game %016x has no version", game.ID)
		}
		for _, v := range game.Versions {
			if v.Kind > StandaloneUpdate {
				return errors.New("a version of an unknown kind")
			}
			if v.Kind == Dlc && (v.Addon == 0 || v.Program != 0) {
				return errors.New("a DLC needs its title ID, and no program")
			}
			if v.Kind != Dlc && v.Program == 0 {
				return errors.New("a game or an update needs its program")
			}
			if len(v.BuildID) != 0 && len(v.BuildID) != 32 {
				return errors.New("a build ID is 32 bytes")
			}
			paths := map[string]bool{}
			for _, f := range v.Files {
				if !safePath(f.Path) || paths[f.Path] {
					return fmt.Errorf("the path %q cannot be written", f.Path)
				}
				paths[f.Path] = true
			}
			for _, d := range v.Dirs {
				if !safePath(d) {
					return fmt.Errorf("the folder %q cannot be written", d)
				}
			}
		}
	}
	return nil
}

func safePath(path string) bool {
	if path == "" || len(path) > 4096*16 || !utf8.ValidString(path) {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 4096 || strings.ContainsAny(part, "\\\x00") {
			return false
		}
	}
	return true
}

func align(n uint64, to uint64) uint64 {
	return (n + to - 1) / to * to
}

func (w *writer) zeros(n int) error {
	_, err := w.buffered.Write(make([]byte, n))
	w.offset += uint64(n)
	return err
}

// chunk is a piece of a file on its way to the file: its bytes, and how it is stored.
type chunk struct {
	data   []byte
	sha    [32]byte
	id     uint32
	stored []byte
	codec  byte
	checks []uint32
}

// addFile stores the blocks of a file and gives its record, the one of the same file at the same
// path in another version of the game when it is the same.
func (w *writer) addFile(key string, f File) (int, error) {
	source, err := os.Open(f.Source)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() != f.Size {
		return 0, fmt.Errorf("%s changed while it was read", f.Path)
	}
	reader := bufio.NewReaderSize(source, 1<<20)
	log2 := byte(bigChunk)
	// a file whose start compresses is cut in pieces of 64 KiB, for reads that decompress less
	if f.Size > 0 {
		start, _ := reader.Peek(1 << smallChunk)
		if len(start) > 0 && w.compress(start) != nil {
			log2 = smallChunk
		}
	}
	size := int64(1) << log2
	record := fileRecord{size: uint64(f.Size), log2: log2}
	hash := sha256.New()
	left := f.Size
	batch := []*chunk{}
	flush := func() error {
		if err := w.store(batch); err != nil {
			return err
		}
		for _, c := range batch {
			record.blocks = append(record.blocks, c.id)
		}
		batch = batch[:0]
		return nil
	}
	for left > 0 {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		n := min(size, left)
		data := make([]byte, n)
		if _, err := io.ReadFull(reader, data); err != nil {
			return 0, fmt.Errorf("%s: %w", f.Path, err)
		}
		hash.Write(data)
		batch = append(batch, &chunk{data: data, sha: sha256.Sum256(data)})
		left -= n
		if len(batch) >= w.workers*2 {
			if err := flush(); err != nil {
				return 0, err
			}
		}
	}
	if err := flush(); err != nil {
		return 0, err
	}
	copy(record.sha[:], hash.Sum(nil))
	record.key = fmt.Sprintf("%s|%d|%x|%d", key, record.size, record.sha, record.log2)
	w.result.Size += f.Size
	if index, ok := w.byKey[record.key]; ok && sameBlocks(w.files[index].blocks, record.blocks) {
		return index, nil
	}
	w.result.Files++
	record.first = uint32(len(w.refs))
	record.count = uint32(len(record.blocks))
	w.refs = append(w.refs, record.blocks...)
	w.files = append(w.files, record)
	w.byKey[record.key] = len(w.files) - 1
	return len(w.files) - 1, nil
}

func sameBlocks(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// compress gives the zstd frame of data when it saves at least a quarter of it, else nil.
func (w *writer) compress(data []byte) []byte {
	frame := w.encoder.EncodeAll(data, make([]byte, 0, len(data)))
	if float64(len(frame)) > float64(len(data))*keepRatio {
		return nil
	}
	return frame
}

// store gives each chunk its block: one stored already with the same bytes, else a new one,
// compressed by the workers and written in order.
func (w *writer) store(batch []*chunk) error {
	fresh := []*chunk{}
	// a chunk with the same bytes as one before it in the batch takes its block once written
	twin := map[*chunk]*chunk{}
	inBatch := map[[32]byte]*chunk{}
	for _, c := range batch {
		if id, ok, err := w.find(c); err != nil {
			return err
		} else if ok {
			c.id = id
			continue
		}
		if first, ok := inBatch[c.sha]; ok && bytes.Equal(first.data, c.data) {
			twin[c] = first
			continue
		}
		inBatch[c.sha] = c
		fresh = append(fresh, c)
	}
	var wg sync.WaitGroup
	lanes := make(chan struct{}, w.workers)
	for _, c := range fresh {
		wg.Add(1)
		lanes <- struct{}{}
		go func(c *chunk) {
			defer wg.Done()
			defer func() { <-lanes }()
			for k := 0; k*checkSize < len(c.data); k++ {
				c.checks = append(c.checks, crc32.Checksum(c.data[k*checkSize:min((k+1)*checkSize, len(c.data))], castagnoli))
			}
			if frame := w.compress(c.data); frame != nil {
				c.stored, c.codec = frame, 1
			} else {
				c.stored, c.codec = c.data, 0
			}
		}(c)
	}
	wg.Wait()
	for _, c := range fresh {
		if pad := align(w.offset, 16) - w.offset; pad > 0 {
			if err := w.zeros(int(pad)); err != nil {
				return err
			}
		}
		b := block{offset: w.offset, stored: uint32(len(c.stored)), logical: uint32(len(c.data)), codec: c.codec, firstCheck: uint32(len(w.checks))}
		if _, err := w.buffered.Write(c.stored); err != nil {
			return err
		}
		w.offset += uint64(len(c.stored))
		c.id = uint32(len(w.blocks))
		w.blocks = append(w.blocks, b)
		w.checks = append(w.checks, c.checks...)
		w.known[c.sha] = append(w.known[c.sha], c.id)
	}
	for _, c := range batch {
		if first, ok := twin[c]; ok {
			c.id = first.id
		}
	}
	for _, c := range batch {
		w.done += int64(len(c.data))
		c.data, c.stored = nil, nil
	}
	if w.progress != nil {
		w.progress(w.done, w.total)
	}
	return nil
}

// find is a block stored already with the bytes of the chunk: the SHA-256 finds it, the bytes read
// back decide.
func (w *writer) find(c *chunk) (uint32, bool, error) {
	for _, id := range w.known[c.sha] {
		b := w.blocks[id]
		if b.logical != uint32(len(c.data)) {
			continue
		}
		if err := w.buffered.Flush(); err != nil {
			return 0, false, err
		}
		stored := make([]byte, b.stored)
		if _, err := w.out.ReadAt(stored, int64(b.offset)); err != nil {
			return 0, false, err
		}
		data := stored
		if b.codec == 1 {
			var err error
			if data, err = w.decoder.DecodeAll(stored, nil); err != nil {
				return 0, false, err
			}
		}
		if bytes.Equal(data, c.data) {
			return id, true, nil
		}
	}
	return 0, false, nil
}
