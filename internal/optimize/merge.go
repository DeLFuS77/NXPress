//go:build cgo

package optimize

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/optimize/native"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Built tells this build has Optimize for install.
const Built = true

func readCnmt(header, section []byte) (uint64, uint8, uint32, []cnmtContent, error) {
	c, err := native.ReadCnmt(header, section)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	out := make([]cnmtContent, 0, len(c.Contents))
	for _, x := range c.Contents {
		out = append(out, cnmtContent{id: hex.EncodeToString(x.ID[:]), typ: x.Type, idOffset: x.IDOffset})
	}
	return c.TitleID, c.Type, c.Version, out, nil
}

func sectionOf(which int, n *nca, i int) native.Section {
	s := native.Section{NCA: which}
	copy(s.Key[:], n.key)
	copy(s.FsHeader[:], n.fsHeader(i))
	s.Start, s.End, _ = n.section(i)
	return s
}

const chunk = 4 << 20

// romfsHash reads a whole RomFS as the game reads it, every block checked by its IVFC, and gives
// its SHA-256.
func romfsHash(ctx context.Context, r *native.Romfs, step string, progress func(string, int64, int64)) ([32]byte, error) {
	h := sha256.New()
	buf := make([]byte, chunk)
	size := r.Size()
	for off := uint64(0); off < size; off += chunk {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, err
		}
		n := int(min(uint64(chunk), size-off))
		if _, err := r.ReadAt(buf[:n], int64(off)); err != nil && !(errors.Is(err, io.EOF) && off+uint64(n) == size) {
			return [32]byte{}, err
		}
		h.Write(buf[:n])
		progress(step, int64(off)+int64(n), int64(size))
	}
	if r.Damaged() != 0 {
		return [32]byte{}, fmt.Errorf("%d blocks of the RomFS do not match their hashes", r.Damaged())
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// Run optimizes the game and its newest update among the files given (NSP, NSZ, XCI or XCZ, a
// SuperNSP too) into one NSP at target, checked whole before it is told done: on an error, target
// is removed. A game alone, with no update newer than it (one merged already, by this or by
// hacBrewPackLZ, or one never updated), is made again the same way from its own program.
func Run(ctx context.Context, files []string, target string, o Options) (res Result, err error) {
	o, err = o.checked()
	if err != nil {
		return res, err
	}
	k, err := loadKeys()
	if err != nil {
		return res, err
	}
	work := o.Work
	if work == "" {
		work = filepath.Dir(target)
	}
	pkgs := []*pkgFile{}
	temps := []string{}
	// closed before the NSP made of each file is removed: Windows does not remove an open file
	defer func() {
		for _, p := range pkgs {
			p.Close()
		}
		for _, t := range temps {
			os.Remove(t)
		}
	}()
	for _, file := range files {
		nsp, temp, err := asNsp(ctx, file, work, func(done, total int64) { o.Progress("Decompressing", done, total) })
		temps = append(temps, temp...)
		if err != nil {
			return res, fmt.Errorf("%s: %w", filepath.Base(file), err)
		}
		p, err := openNsp(nsp)
		if err != nil {
			return res, err
		}
		pkgs = append(pkgs, p)
	}
	g, err := readGame(k, pkgs, readCnmt)
	if err != nil {
		return res, err
	}
	base, update, err := g.pair()
	if err != nil {
		return res, ErrNotPair{err.Error()}
	}
	// a game alone (merged already, or with no update newer than it) is made again from its own
	// program, laid out as Nintendo lays it out
	alone := update == nil
	res.Alone = alone
	// what the new game is made of: the update's contents, or the game's own
	from, of := update, "the update"
	if alone {
		from, of = base, "the game"
	}
	bprog, bn, err := g.program(base, 0)
	if err != nil {
		return res, ErrNotPair{err.Error()}
	}
	uprog, un := bprog, 0
	if !alone {
		if uprog, un, err = g.program(update, 0); err != nil {
			return res, ErrNotPair{err.Error()}
		}
	}
	if bn > 1 || un > 1 {
		return res, ErrNotPair{"a game of more than one program is not optimized yet"}
	}
	titleKeys := map[string][]byte{}
	for r, t := range g.tickets {
		titleKeys[r] = t.titleKey
	}
	if err := bprog.unlock(k, titleKeys); err != nil {
		return res, err
	}
	if !alone {
		if err := uprog.unlock(k, titleKeys); err != nil {
			return res, err
		}
	}
	bi, ui := bprog.romfsIndex(), uprog.romfsIndex()
	if bi < 0 || ui < 0 {
		return res, ErrNotPair{"the program of the game or of its update has no RomFS"}
	}
	res.TitleID, res.Version, res.BaseVersion = base.titleID, from.version, base.version
	res.ProgramBefore = bprog.size
	metas := []*meta{base}
	if !alone {
		res.ProgramBefore += uprog.size
		metas = append(metas, update)
	}
	for _, m := range metas {
		res.InputSize += m.nca.size
		for _, c := range m.contents {
			if n := g.ncas[c.id]; n != nil {
				res.InputSize += n.size
			}
		}
	}

	stepped := func(step string, total uint64) func(int, uint64) bool {
		return func(_ int, done uint64) bool {
			o.Progress(step, int64(done), int64(total))
			return ctx.Err() == nil
		}
	}
	job := &native.Job{NCAs: []io.ReaderAt{bprog, uprog}}
	job.EncryptHeader = func(h []byte) {
		if err := xts(k.header, h, true); err != nil {
			panic(err)
		}
	}
	bsec, usec := sectionOf(0, bprog, bi), sectionOf(1, uprog, ui)

	// 1. the RomFS of the game with its update (or alone), as the game reads it: what the new one
	// must give
	var r *native.Romfs
	if alone {
		r, err = native.OpenRomfs(job, bsec, nil)
	} else {
		r, err = native.OpenRomfs(job, bsec, &usec)
	}
	if err != nil {
		if alone {
			return res, ErrNotPair{"the RomFS of the game is not read: " + err.Error()}
		}
		return res, ErrNotPair{"the update does not read over this base game: " + err.Error()}
	}
	want, err := romfsHash(ctx, r, "Reading", o.Progress)
	r.Close()
	if err != nil {
		return res, err
	}

	// 2. the merged program, written as it is packed (one pass), or measured and written again
	var m *native.Merger
	if alone {
		m, err = native.OpenAlone(job, bsec, bi, bprog.header, o.Level, o.Threads)
	} else {
		m, err = native.Open(job, bsec, usec, ui, uprog.header, o.Level, o.Threads)
	}
	if err != nil {
		return res, ErrNotPair{err.Error()}
	}
	defer m.Close()

	// the files of the NSP: the merged program, the other contents of the update (or of the game
	// alone; no delta fragments), the meta NCA, and the tickets of what has a rights ID
	others := []*nca{}
	for _, c := range from.contents {
		if c.typ == 1 || c.typ == 6 {
			continue
		}
		n := g.ncas[c.id]
		if n == nil {
			return res, fmt.Errorf("%s.nca of %s is not among the files given", c.id, of)
		}
		others = append(others, n)
	}
	rights := []string{}
	seen := map[string]bool{}
	for _, n := range append([]*nca{uprog}, others...) {
		if !n.hasRights() {
			continue
		}
		r := hex.EncodeToString(n.rightsID())
		if seen[r] {
			continue
		}
		seen[r] = true
		if g.tickets[r] == nil {
			return res, fmt.Errorf("%s has no ticket for %s (a title key of title.keys is not enough for an NSP to install)", of, n.name)
		}
		rights = append(rights, r)
	}
	zeros := strings.Repeat("0", 32)
	names := []string{zeros + ".nca"}
	for _, n := range others {
		names = append(names, strings.ToLower(n.name))
	}
	metaAt := len(names)
	names = append(names, zeros+".cnmt.nca")
	for _, r := range rights {
		names = append(names, r+".tik")
		if g.tickets[r].cert != nil {
			names = append(names, r+".cert")
		}
	}

	f, err := switchfs.CreateNew(target)
	if err != nil {
		return res, err
	}
	done := false
	defer func() {
		f.Close()
		if !done {
			os.Remove(target)
		}
	}()
	pw, err := newPfsWriter(f, names)
	if err != nil {
		return res, err
	}
	pw.next()
	info, sum, once, err := writeProgram(ctx, f, pw, job, m, !o.twoPasses, stepped)
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		return res, err
	}
	res.OnePass = once
	var program native.Content
	copy(program.Hash[:], sum)
	copy(program.ID[:], program.Hash[:16])
	program.Size, program.Type = info.NcaSize, native.ContentProgram
	res.ProgramID = hex.EncodeToString(program.ID[:])
	res.ProgramHash = hex.EncodeToString(program.Hash[:])
	if err := pw.rename(0, res.ProgramID+".nca"); err != nil {
		return res, err
	}
	for _, n := range others {
		pw.next()
		if err := copyN(pw, n.file, n.offset, n.size); err != nil {
			return res, err
		}
	}

	// the meta NCA: made from the one of the base, its section encrypted with its key at 0xC00
	var mm native.MergedMeta
	if alone {
		mm, err = native.MakeMergedMeta(base.nca.header, base.section, nil, nil, program)
	} else {
		mm, err = native.MakeMergedMeta(base.nca.header, base.section, update.nca.header, update.section, program)
	}
	if err != nil {
		return res, err
	}
	if err := sameContents(mm.Contents, program, others); err != nil {
		return res, err
	}
	if mm.Header[0x400+4] == 3 {
		if err := ctrCrypt(base.nca.key, mm.Header[0x400:0x600], 0xC00, mm.Section); err != nil {
			return res, err
		}
	}
	if err := xts(k.header, mm.Header, true); err != nil {
		return res, err
	}
	metaNca := append(mm.Header, mm.Section...)
	metaSum := sha256.Sum256(metaNca)
	if err := pw.rename(metaAt, hex.EncodeToString(metaSum[:16])+".cnmt.nca"); err != nil {
		return res, err
	}
	pw.next()
	if _, err := pw.Write(metaNca); err != nil {
		return res, err
	}
	for _, r := range rights {
		pw.next()
		if _, err := pw.Write(g.tickets[r].tik); err != nil {
			return res, err
		}
		if g.tickets[r].cert != nil {
			pw.next()
			if _, err := pw.Write(g.tickets[r].cert); err != nil {
				return res, err
			}
		}
	}
	if err := pw.finish(); err != nil {
		return res, err
	}
	if err := f.Sync(); err != nil {
		return res, err
	}
	res.ProgramAfter = int64(info.NcaSize)
	res.RomfsSize, res.RunsAgain, res.BlocksAgain, res.OldKept = info.RomfsSize, info.RunsAgain, info.BlocksAgain, info.OldKept
	res.OutputSize = pw.head + pw.at

	// 3. checked whole, opened again from the disk
	if err := CheckMerged(ctx, target, want, o.Progress); err != nil {
		return res, fmt.Errorf("the new NSP does not check: %w", err)
	}
	done = true
	return res, nil
}

// offsetWriter writes in order from a place of a file, without moving its offset.
type offsetWriter struct {
	f  *os.File
	at int64
}

func (w *offsetWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.at)
	w.at += int64(n)
	return n, err
}

// writeProgram writes the merged program NCA as the first file of the NSP pw writes to f, and
// gives what was measured and its SHA-256. In one pass when it can (once): the data level of the
// RomFS goes to its place as it is packed, then the rest of the NCA around it, and the NCA is read
// back for its hash. When what is stored came out bigger than the RomFS, it is not where it was
// written: the NCA is written again from its start, in two passes, as without once.
func writeProgram(ctx context.Context, f *os.File, pw *pfsWriter, job *native.Job, m *native.Merger, once bool,
	stepped func(string, uint64) func(int, uint64) bool) (native.Info, []byte, bool, error) {
	romfs := m.Info().RomfsSize
	once = once && m.OnceBound() > 0
	dataLen := uint64(0)
	if once {
		// the head of the NSP on the disk: the NCA goes after it
		if err := pw.w.Flush(); err != nil {
			return native.Info{}, nil, false, err
		}
		data := bufio.NewWriterSize(&offsetWriter{f: f, at: pw.head + int64(m.DataPlace())}, chunk)
		job.Out = func(p []byte) error {
			_, err := data.Write(p)
			dataLen += uint64(len(p))
			return err
		}
		job.Step = stepped("Writing", romfs)
		if err := m.MeasureOnce(); err != nil {
			return native.Info{}, nil, false, err
		}
		if err := data.Flush(); err != nil {
			return native.Info{}, nil, false, err
		}
		if !m.Placed() {
			// what it stores is bigger than the RomFS: what was written goes, written again in order
			once = false
			if err := f.Truncate(pw.head); err != nil {
				return native.Info{}, nil, false, err
			}
			if _, err := f.Seek(pw.head, io.SeekStart); err != nil {
				return native.Info{}, nil, false, err
			}
		}
	} else {
		job.Step = stepped("Measuring", romfs)
		if err := m.Measure(); err != nil {
			return native.Info{}, nil, false, err
		}
	}
	info := m.Info()
	if once {
		// each part written, [from, to) of the NCA: with the data level, the whole NCA once each
		parts := [][2]uint64{{m.DataPlace(), m.DataPlace() + dataLen}}
		var part *bufio.Writer
		job.Begin = func(at uint64) error {
			if part != nil {
				if err := part.Flush(); err != nil {
					return err
				}
			}
			part = bufio.NewWriterSize(&offsetWriter{f: f, at: pw.head + int64(at)}, chunk)
			parts = append(parts, [2]uint64{at, at})
			return nil
		}
		job.Out = func(p []byte) error {
			if part == nil {
				return errors.New("a part of the NCA with no place")
			}
			_, err := part.Write(p)
			parts[len(parts)-1][1] += uint64(len(p))
			return err
		}
		err := m.WriteAround()
		job.Begin = nil
		if err != nil {
			return info, nil, true, err
		}
		if part == nil {
			return info, nil, true, errors.New("nothing written around the data level")
		}
		if err := part.Flush(); err != nil {
			return info, nil, true, err
		}
		if err := partsWhole(parts, info.NcaSize); err != nil {
			return info, nil, true, err
		}
		// the NCA whole: the files after it go from its end
		pw.sizes[0], pw.at = int64(info.NcaSize), int64(info.NcaSize)
		if _, err := f.Seek(pw.head+int64(info.NcaSize), io.SeekStart); err != nil {
			return info, nil, true, err
		}
		pw.w.Reset(f)
		// its hash, read back (f is opened only to write)
		r, err := os.Open(f.Name())
		if err != nil {
			return info, nil, true, err
		}
		defer r.Close()
		whole := sha256.New()
		buf := make([]byte, chunk)
		for off := uint64(0); off < info.NcaSize; {
			if err := ctx.Err(); err != nil {
				return info, nil, true, err
			}
			n := int(min(uint64(len(buf)), info.NcaSize-off))
			if _, err := r.ReadAt(buf[:n], pw.head+int64(off)); err != nil {
				return info, nil, true, err
			}
			whole.Write(buf[:n])
			off += uint64(n)
		}
		return info, whole.Sum(nil), true, nil
	}
	var whole hash.Hash = sha256.New()
	job.Out = func(p []byte) error {
		whole.Write(p)
		_, err := pw.Write(p)
		return err
	}
	job.Step = stepped("Writing", romfs)
	if err := m.Write(); err != nil {
		return info, nil, false, err
	}
	if uint64(pw.sizes[0]) != info.NcaSize {
		return info, nil, false, fmt.Errorf("the merged NCA was written with %d bytes, measured %d", pw.sizes[0], info.NcaSize)
	}
	return info, whole.Sum(nil), false, nil
}

// partsWhole checks the parts written of an NCA of size bytes, [from, to) each, make it whole:
// none over another, no gap.
func partsWhole(parts [][2]uint64, size uint64) error {
	sorted := slices.Clone(parts)
	slices.SortFunc(sorted, func(a, b [2]uint64) int { return cmp.Compare(a[0], b[0]) })
	at := uint64(0)
	for _, p := range sorted {
		if p[1] == p[0] {
			continue
		}
		if p[0] != at || p[1] < p[0] {
			return fmt.Errorf("the merged NCA was not written whole in one pass: a part of %d to %d where %d was next", p[0], p[1], at)
		}
		at = p[1]
	}
	if at != size {
		return fmt.Errorf("the merged NCA was written in one pass up to %d bytes, measured %d", at, size)
	}
	return nil
}

// sameContents checks the CNMT made lists the files of the NSP.
func sameContents(listed []native.Content, program native.Content, others []*nca) error {
	want := map[string]bool{hex.EncodeToString(program.ID[:]): true}
	for _, n := range others {
		want[strings.TrimSuffix(strings.ToLower(n.name), ".nca")] = true
	}
	if len(listed) != len(want) {
		return fmt.Errorf("the new CNMT lists %d contents, the NSP holds %d", len(listed), len(want))
	}
	for _, c := range listed {
		if !want[hex.EncodeToString(c.ID[:])] {
			return fmt.Errorf("the new CNMT lists %x, which the NSP does not hold", c.ID)
		}
		if c.Type == native.ContentProgram && (c.Hash != program.Hash || c.Size != program.Size) {
			return errors.New("the new CNMT does not give the hash and the size of the merged program")
		}
	}
	return nil
}

// plain reads [off, off+size) of section i of an NCA decrypted.
func plain(n *nca, i int, off uint64, size int) ([]byte, error) {
	start, end, _ := n.section(i)
	if off+uint64(size) > end-start {
		return nil, errors.New("a read past its section")
	}
	from := off &^ 15
	buf := make([]byte, (int(off-from)+size+15)&^15)
	if from+uint64(len(buf)) > end-start {
		buf = buf[:end-start-from]
	}
	if _, err := n.ReadAt(buf, int64(start+from)); err != nil {
		return nil, err
	}
	if n.fsHeader(i)[4] == 3 {
		if err := ctrCrypt(n.key, n.fsHeader(i), start+from, buf); err != nil {
			return nil, err
		}
	}
	return buf[off-from : int(off-from)+size], nil
}

// CheckMerged opens an optimized NSP again from the disk and checks it whole: the CNMT of its
// meta NCA lists its files with their sizes and the hash of the program; the program NCA: the
// hashes of its FS headers, every block of its PFS0 sections, and its RomFS read as the game
// reads it (every block by its IVFC, the LZ4 blocks decompressed) giving the SHA-256 want; then
// every NCA by its content ID (switchfs.VerifyFile).
func CheckMerged(ctx context.Context, path string, want [32]byte, progress func(string, int64, int64)) error {
	if progress == nil {
		progress = func(string, int64, int64) {}
	}
	k, err := loadKeys()
	if err != nil {
		return err
	}
	p, err := openNsp(path)
	if err != nil {
		return err
	}
	defer p.Close()
	g, err := readGame(k, []*pkgFile{p}, readCnmt)
	if err != nil {
		return err
	}
	if len(g.metas) != 1 || g.metas[0].typ != metaApplication {
		return errors.New("it does not hold one Application")
	}
	m := g.metas[0]
	listed := map[string]bool{}
	for _, c := range m.contents {
		listed[c.id] = true
	}
	for id := range g.ncas {
		if id != strings.TrimSuffix(strings.ToLower(m.nca.name), ".cnmt.nca") && !listed[id] {
			return fmt.Errorf("%s.nca is not listed in its CNMT", id)
		}
	}
	prog, programs, err := g.program(m, 0)
	if err != nil {
		return err
	}
	if programs != 1 {
		return errors.New("it holds more than one program")
	}
	titleKeys := map[string][]byte{}
	for r, t := range g.tickets {
		titleKeys[r] = t.titleKey
	}
	if err := prog.unlock(k, titleKeys); err != nil {
		return err
	}
	if binary.LittleEndian.Uint64(prog.header[0x208:]) != uint64(prog.size) {
		return errors.New("the size in the header of the program is not its size")
	}
	romfsChecked := false
	for i := range 4 {
		start, end, ok := prog.section(i)
		if !ok {
			continue
		}
		fsh := prog.fsHeader(i)
		sum := sha256.Sum256(fsh)
		if !bytes.Equal(sum[:], prog.header[0x280+i*0x20:0x2A0+i*0x20]) {
			return fmt.Errorf("the hash of FS header %d is wrong", i)
		}
		switch {
		case fsh[3] == 2: // HierarchicalSha256: the master hash, then each block
			bs := binary.LittleEndian.Uint32(fsh[8+0x20:])
			ho, hs := binary.LittleEndian.Uint64(fsh[8+0x28:]), binary.LittleEndian.Uint64(fsh[8+0x30:])
			po, ps := binary.LittleEndian.Uint64(fsh[8+0x38:]), binary.LittleEndian.Uint64(fsh[8+0x40:])
			if bs == 0 || hs > 64<<20 || ho+hs > end-start || po+ps > end-start {
				return fmt.Errorf("section %d is not a hash table and its data", i)
			}
			table, err := plain(prog, i, ho, int(hs))
			if err != nil {
				return err
			}
			if master := sha256.Sum256(table); !bytes.Equal(master[:], fsh[8:8+32]) {
				return fmt.Errorf("the master hash of section %d is wrong", i)
			}
			for b := uint64(0); b*uint64(bs) < ps; b++ {
				if err := ctx.Err(); err != nil {
					return err
				}
				n := min(uint64(bs), ps-b*uint64(bs))
				data, err := plain(prog, i, po+b*uint64(bs), int(n))
				if err != nil {
					return err
				}
				if sum := sha256.Sum256(data); (b+1)*32 > uint64(len(table)) || !bytes.Equal(sum[:], table[b*32:b*32+32]) {
					return fmt.Errorf("block %d of section %d does not match its hash", b, i)
				}
			}
		case fsh[2] == 0: // the RomFS
			job := &native.Job{NCAs: []io.ReaderAt{prog}}
			r, err := native.OpenRomfs(job, sectionOf(0, prog, i), nil)
			if err != nil {
				return err
			}
			got, err := romfsHash(ctx, r, "Checking", progress)
			r.Close()
			if err != nil {
				return err
			}
			if got != want {
				return errors.New("its RomFS is not the one of the game with its update")
			}
			romfsChecked = true
		default:
			return fmt.Errorf("section %d is of a kind not checked", i)
		}
	}
	if !romfsChecked {
		return errors.New("the program has no RomFS")
	}
	check, err := switchfs.VerifyFile(ctx, path, func(done, total int64) { progress("Verifying", done, total) })
	if err != nil {
		return err
	}
	if len(check.Modified) > 0 || check.Partial {
		return fmt.Errorf("%s do not match their content IDs", strings.Join(check.Modified, ", "))
	}
	return nil
}
