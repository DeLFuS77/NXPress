//go:build cgo

// Package native runs the code of NXDock that merges a game with its update into one program NCA
// (patch_view, merge_pack, merge_nca, merge_game, merge_meta, romfs_lz4 and lz4nintendo, copied
// from NXDock by tools/nxdocksync), with the reads, the AES, the SHA-256 and the writes of Go
// (bridge.h). It is built only with cgo.
package native

/*
#cgo CFLAGS: -O2
#cgo CXXFLAGS: -std=c++20 -O2
#cgo windows LDFLAGS: -static
#cgo linux LDFLAGS: -lpthread
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"io"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// Section is one section of an NCA: which NCA it is (an index of Job.NCAs), its key, its FS header
// and where it is in the NCA.
type Section struct {
	NCA        int
	Key        [16]byte
	FsHeader   [0x200]byte
	Start, End uint64
}

// Job is what one merge reads, writes and tells.
type Job struct {
	// the NCAs read, as they are stored (encrypted), from their start
	NCAs []io.ReaderAt
	// the AES-XTS of an NCA header, in place
	EncryptHeader func(header []byte)
	// the new NCA, in order from its start
	Out func(p []byte) error
	// the progress of each phase (Measure, Write): the bytes of the RomFS done; false stops
	Step func(phase int, done uint64) bool

	mu     sync.Mutex
	failed error // the first error of a read or a write, told instead of the one of C
}

// The phases of Job.Step.
const (
	Measure = int(C.NXO_MEASURE)
	Write   = int(C.NXO_WRITE)
)

func (j *Job) fail(err error) {
	j.mu.Lock()
	if j.failed == nil {
		j.failed = err
	}
	j.mu.Unlock()
}

func (j *Job) err(cerr []byte) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.failed != nil {
		return j.failed
	}
	n := 0
	for n < len(cerr) && cerr[n] != 0 {
		n++
	}
	if n == 0 {
		return errors.New("failed")
	}
	return errors.New(string(cerr[:n]))
}

func cSection(s Section) C.nxo_section {
	var c C.nxo_section
	c.which = C.int(s.NCA)
	for i := range s.Key {
		c.key[i] = C.uint8_t(s.Key[i])
	}
	for i := range s.FsHeader {
		c.fsh[i] = C.uint8_t(s.FsHeader[i])
	}
	c.start = C.uint64_t(s.Start)
	c.size = C.uint64_t(s.End - s.Start)
	return c
}

const errLen = 512

// Info is what a merge measured.
type Info struct {
	RomfsSize, StoredSize, NcaSize uint64
	RunsAgain, BlocksAgain         uint64
	OldKept, Damaged               uint64
}

// Merger is the program NCA of a game merged with its update.
type Merger struct {
	job    *Job
	handle cgo.Handle
	m      *C.nxo_merger
}

// Open opens the RomFS sections of the base program and of the update program over it: an error
// when they are not a pair NXDock reads (the base of another version, a damaged one).
func Open(job *Job, base Section, update Section, updateIndex int, updateHeader []byte, level int, threads int) (*Merger, error) {
	if len(updateHeader) < 0xC00 {
		return nil, errors.New("not an NCA header")
	}
	m := &Merger{job: job, handle: cgo.NewHandle(job)}
	m.m = C.nxo_merger_new(C.uintptr_t(m.handle))
	if m.m == nil {
		m.handle.Delete()
		return nil, errors.New("out of memory")
	}
	cb, cu := cSection(base), cSection(update)
	header := C.CBytes(updateHeader[:0xC00])
	defer C.free(header)
	cerr := make([]byte, errLen)
	if C.nxo_merger_open(m.m, &cb, &cu, C.int(updateIndex), (*C.uint8_t)(header), C.int(level), C.unsigned(threads),
		(*C.char)(unsafe.Pointer(&cerr[0])), errLen) == 0 {
		err := job.err(cerr)
		m.Close()
		return nil, err
	}
	return m, nil
}

// Measure packs the RomFS once: after it, the size of the NCA is known.
func (m *Merger) Measure() error {
	cerr := make([]byte, errLen)
	if C.nxo_merger_measure(m.m, (*C.char)(unsafe.Pointer(&cerr[0])), errLen) == 0 {
		return m.job.err(cerr)
	}
	return nil
}

// Write packs it again and gives the whole NCA to Job.Out, in order; what it stores must hash as
// it did when measured, or it fails.
func (m *Merger) Write() error {
	cerr := make([]byte, errLen)
	if C.nxo_merger_write(m.m, (*C.char)(unsafe.Pointer(&cerr[0])), errLen) == 0 {
		return m.job.err(cerr)
	}
	return nil
}

// Info tells the sizes and what was stored once.
func (m *Merger) Info() Info {
	var c C.nxo_info
	C.nxo_merger_info(m.m, &c)
	return Info{
		RomfsSize: uint64(c.romfs_size), StoredSize: uint64(c.stored_size), NcaSize: uint64(c.nca_size),
		RunsAgain: uint64(c.runs_again), BlocksAgain: uint64(c.blocks_again), OldKept: uint64(c.old_kept), Damaged: uint64(c.damaged),
	}
}

// Close frees it.
func (m *Merger) Close() {
	if m.m != nil {
		C.nxo_merger_free(m.m)
		m.m = nil
		m.handle.Delete()
	}
}

// Romfs is a RomFS as the game reads it, each block checked against its IVFC.
type Romfs struct {
	job    *Job
	handle cgo.Handle
	r      *C.nxo_romfs
}

// OpenRomfs opens the RomFS of a section, or of an update over its base (update not nil).
func OpenRomfs(job *Job, base Section, update *Section) (*Romfs, error) {
	r := &Romfs{job: job, handle: cgo.NewHandle(job)}
	cb := cSection(base)
	var cu *C.nxo_section
	if update != nil {
		u := cSection(*update)
		cu = &u
	}
	cerr := make([]byte, errLen)
	r.r = C.nxo_romfs_open(C.uintptr_t(r.handle), &cb, cu, (*C.char)(unsafe.Pointer(&cerr[0])), errLen)
	if r.r == nil {
		r.handle.Delete()
		return nil, job.err(cerr)
	}
	return r, nil
}

func (r *Romfs) Size() uint64     { return uint64(C.nxo_romfs_size(r.r)) }
func (r *Romfs) Stored() uint64   { return uint64(C.nxo_romfs_stored(r.r)) }
func (r *Romfs) Compressed() bool { return C.nxo_romfs_compressed(r.r) != 0 }
func (r *Romfs) Damaged() uint64  { return uint64(C.nxo_romfs_damaged(r.r)) }
func (r *Romfs) Close()           { C.nxo_romfs_free(r.r); r.handle.Delete() }

// ReadAt reads the RomFS as the game reads it; a block that does not match its hash fails.
func (r *Romfs) ReadAt(p []byte, offset int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	size := r.Size()
	if offset < 0 || uint64(offset) >= size {
		return 0, io.EOF
	}
	n := len(p)
	if uint64(offset)+uint64(n) > size {
		n = int(size - uint64(offset))
	}
	buf := C.malloc(C.size_t(n))
	defer C.free(buf)
	if C.nxo_romfs_read(r.r, C.uint64_t(offset), (*C.uint8_t)(buf), C.size_t(n)) == 0 {
		r.job.mu.Lock()
		failed := r.job.failed
		r.job.mu.Unlock()
		if failed != nil {
			return 0, failed
		}
		return 0, errors.New("the RomFS does not match its hashes")
	}
	copy(p, unsafe.Slice((*byte)(buf), n))
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Content is one content of a CNMT.
type Content struct {
	ID       [16]byte
	Hash     [32]byte
	Size     uint64
	Type     uint8
	IDOffset uint8
}

// Content types of a CNMT.
const (
	ContentMeta          = 0
	ContentProgram       = 1
	ContentControl       = 3
	ContentDeltaFragment = 6
)

func contents(c *C.nxo_content, n C.size_t) []Content {
	out := make([]Content, 0, int(n))
	for _, e := range unsafe.Slice(c, int(n)) {
		var x Content
		for i := range x.ID {
			x.ID[i] = byte(e.id[i])
		}
		for i := range x.Hash {
			x.Hash[i] = byte(e.hash[i])
		}
		x.Size, x.Type, x.IDOffset = uint64(e.size), uint8(e._type), uint8(e.id_offset)
		out = append(out, x)
	}
	return out
}

// Cnmt is what a CNMT tells.
type Cnmt struct {
	TitleID  uint64
	Type     uint8
	Version  uint32
	Contents []Content
}

// ReadCnmt reads the CNMT of a meta NCA: its header decrypted (0xC00) and its section decrypted.
func ReadCnmt(header []byte, section []byte) (Cnmt, error) {
	if len(header) < 0xC00 || len(section) == 0 {
		return Cnmt{}, errors.New("not a meta NCA")
	}
	h := C.CBytes(header[:0xC00])
	defer C.free(h)
	s := C.CBytes(section)
	defer C.free(s)
	var (
		out   Cnmt
		id    C.uint64_t
		typ   C.uint8_t
		ver   C.uint32_t
		list  *C.nxo_content
		count C.size_t
	)
	cerr := make([]byte, errLen)
	if C.nxo_read_cnmt((*C.uint8_t)(h), (*C.uint8_t)(s), C.size_t(len(section)), &id, &typ, &ver, &list, &count,
		(*C.char)(unsafe.Pointer(&cerr[0])), errLen) == 0 {
		return out, (&Job{}).err(cerr)
	}
	defer C.nxo_free(unsafe.Pointer(list))
	out.TitleID, out.Type, out.Version, out.Contents = uint64(id), uint8(typ), uint32(ver), contents(list, count)
	return out, nil
}

// MergedMeta is the meta NCA of a merged game.
type MergedMeta struct {
	Header      []byte // decrypted, 0xC00
	Section     []byte // not encrypted yet, from 0xC00
	TitleID     uint64
	Version     uint32
	BaseVersion uint32
	Contents    []Content
}

// MakeMergedMeta makes the CNMT of the merged game (the base's, with the version and the contents
// of the update, the merged program in place of its program, no delta fragments and no required
// version) and its meta NCA, from the meta NCAs of the base and the update.
func MakeMergedMeta(baseHeader, baseSection, updateHeader, updateSection []byte, program Content) (MergedMeta, error) {
	if len(baseHeader) < 0xC00 || len(updateHeader) < 0xC00 || len(baseSection) == 0 || len(updateSection) == 0 {
		return MergedMeta{}, errors.New("not meta NCAs")
	}
	bh, bs := C.CBytes(baseHeader[:0xC00]), C.CBytes(baseSection)
	uh, us := C.CBytes(updateHeader[:0xC00]), C.CBytes(updateSection)
	defer C.free(bh)
	defer C.free(bs)
	defer C.free(uh)
	defer C.free(us)
	var p C.nxo_content
	for i := range program.ID {
		p.id[i] = C.uint8_t(program.ID[i])
	}
	for i := range program.Hash {
		p.hash[i] = C.uint8_t(program.Hash[i])
	}
	p.size, p._type, p.id_offset = C.uint64_t(program.Size), C.uint8_t(program.Type), C.uint8_t(program.IDOffset)
	header := make([]byte, 0xC00)
	var (
		section *C.uint8_t
		secLen  C.size_t
		list    *C.nxo_content
		count   C.size_t
		id      C.uint64_t
		ver, bv C.uint32_t
	)
	cerr := make([]byte, errLen)
	if C.nxo_merged_meta(0, (*C.uint8_t)(bh), (*C.uint8_t)(bs), C.size_t(len(baseSection)), (*C.uint8_t)(uh), (*C.uint8_t)(us),
		C.size_t(len(updateSection)), &p, (*C.uint8_t)(unsafe.Pointer(&header[0])), &section, &secLen, &list, &count, &id, &ver, &bv,
		(*C.char)(unsafe.Pointer(&cerr[0])), errLen) == 0 {
		return MergedMeta{}, (&Job{}).err(cerr)
	}
	defer C.nxo_free(unsafe.Pointer(section))
	defer C.nxo_free(unsafe.Pointer(list))
	return MergedMeta{
		Header: header, Section: C.GoBytes(unsafe.Pointer(section), C.int(secLen)), TitleID: uint64(id), Version: uint32(ver),
		BaseVersion: uint32(bv), Contents: contents(list, count),
	}, nil
}
