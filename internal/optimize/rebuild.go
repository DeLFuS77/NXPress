//go:build cgo

package optimize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/optimize/native"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Rebuild writes a package again as a clean NSP at target, its data as it is (no compression, no
// LZ4): every NCA with a rights ID moved to standard crypto (its title key put into its key area,
// so no ticket is needed), the hashes of its FS headers made right, its header encrypted again;
// each NCA named by the SHA-256 of what it is now; each CNMT made again with those IDs, hashes
// and sizes, in a new meta NCA; a new PFS0 with the NCA files only. It is checked whole before it
// is told done: on an error, target is removed. See docs/rebuild.md.
func Rebuild(ctx context.Context, file string, target string, o Options) (res RebuildResult, err error) {
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
	nsp, temps, err := asNsp(ctx, file, work, func(done, total int64) { o.Progress("Decompressing", done, total) })
	var p *pkgFile
	defer func() {
		if p != nil {
			p.Close()
		}
		for _, t := range temps {
			os.Remove(t)
		}
	}()
	if err != nil {
		return res, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	if p, err = openNsp(nsp); err != nil {
		return res, err
	}
	if info, err := os.Stat(nsp); err == nil {
		res.InputSize = info.Size()
	}
	g, err := readGame(k, []*pkgFile{p}, readCnmt)
	if err != nil {
		return res, err
	}
	if len(g.metas) == 0 {
		return res, errors.New("it holds no CNMT: there is no title to rebuild")
	}
	titleKeys := map[string][]byte{}
	for r, t := range g.tickets {
		titleKeys[r] = t.titleKey
	}
	metaNames := map[string]bool{}
	deltas := map[string]bool{}
	for _, m := range g.metas {
		metaNames[m.nca.name] = true
		for _, c := range m.contents {
			if c.typ == 6 {
				deltas[c.id] = true
			}
		}
	}
	// the NCA files in the order of the package, then the meta NCAs
	ncas := []*nca{}
	for _, name := range p.order {
		if !strings.HasSuffix(strings.ToLower(name), ".nca") {
			res.Dropped++
			continue
		}
		if metaNames[name] {
			continue
		}
		id := strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(name), ".nca"), ".cnmt")
		if deltas[id] {
			res.Dropped++ // a delta fragment: an installed game never uses it
			continue
		}
		n := g.ncas[id]
		if n == nil {
			return res, fmt.Errorf("%s is not read", name)
		}
		if err := n.unlock(k, titleKeys); err != nil {
			return res, err
		}
		ncas = append(ncas, n)
	}
	res.Titles, res.NCAs = len(g.metas), len(ncas)
	zeros := strings.Repeat("0", 32)
	names := []string{}
	for range ncas {
		names = append(names, zeros+".nca")
	}
	for range g.metas {
		names = append(names, zeros+".cnmt.nca")
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
	changed := map[[16]byte]native.Content{}
	var total, written int64
	for _, n := range ncas {
		total += n.size
	}
	for i, n := range ncas {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		header, standard, fixed, err := cleanHeader(k, n)
		if err != nil {
			return res, fmt.Errorf("%s: %w", n.name, err)
		}
		if standard {
			res.Standard++
		}
		if n.header[0x204] != 0 {
			res.Download++
		}
		res.FsFixed += fixed
		pw.next()
		sum := sha256.New()
		out := io.MultiWriter(pw, sum)
		if _, err := out.Write(header); err != nil {
			return res, err
		}
		// the rest as it is: the sections keep their key and their counters
		body := io.NewSectionReader(n, int64(len(header)), n.size-int64(len(header)))
		if _, err := io.Copy(out, &progressReader{r: body, step: func(k int64) {
			written += k
			o.Progress("Rebuilding", written, total)
		}}); err != nil {
			return res, err
		}
		var c native.Content
		copy(c.Hash[:], sum.Sum(nil))
		copy(c.ID[:], c.Hash[:16])
		c.Size = uint64(n.size)
		newID := hex.EncodeToString(c.ID[:])
		if err := pw.rename(i, newID+".nca"); err != nil {
			return res, err
		}
		var old [16]byte
		hex.Decode(old[:], []byte(strings.TrimSuffix(strings.ToLower(n.name), ".nca")))
		if hex.EncodeToString(old[:]) != newID {
			res.Renamed++
		}
		changed[old] = c
	}
	for j, m := range g.metas {
		header, section, err := native.RebuiltMeta(m.nca.header, m.section, changedFor(m, changed))
		if err != nil {
			return res, fmt.Errorf("%s: %w", m.nca.name, err)
		}
		if header[0x400+4] == 3 {
			if err := ctrCrypt(m.nca.key, header[0x400:0x600], 0xC00, section); err != nil {
				return res, err
			}
		}
		if err := xts(k.header, header, true); err != nil {
			return res, err
		}
		whole := append(header, section...)
		sum := sha256.Sum256(whole)
		if err := pw.rename(len(ncas)+j, hex.EncodeToString(sum[:16])+".cnmt.nca"); err != nil {
			return res, err
		}
		pw.next()
		if _, err := pw.Write(whole); err != nil {
			return res, err
		}
	}
	if err := pw.finish(); err != nil {
		return res, err
	}
	if err := f.Sync(); err != nil {
		return res, err
	}
	res.OutputSize = pw.head + pw.at
	if err := CheckRebuilt(ctx, target, o.Progress); err != nil {
		return res, fmt.Errorf("the new NSP does not check: %w", err)
	}
	done = true
	return res, nil
}

// changedFor is the part of changed a title lists: its contents only.
func changedFor(m *meta, changed map[[16]byte]native.Content) map[[16]byte]native.Content {
	out := map[[16]byte]native.Content{}
	for _, c := range m.contents {
		var id [16]byte
		hex.Decode(id[:], []byte(c.id))
		if v, ok := changed[id]; ok {
			out[id] = v
		}
	}
	return out
}

// cleanHeader is the header of an NCA as Rebuild writes it, encrypted: with standard crypto when
// it had a rights ID (the title key in slot 2 of its key area, encrypted with the key area key of
// an application of its generation; no ticket needed) and the hash of each FS header made right.
func cleanHeader(k *keyring, n *nca) (header []byte, standard bool, fixed int, err error) {
	header = append([]byte(nil), n.header...)
	if n.hasRights() {
		gen := n.generation()
		kak := k.lookup(fmt.Sprintf("key_area_key_application_%02x", gen))
		if len(kak) != 16 {
			return nil, false, 0, fmt.Errorf("prod.keys has no key_area_key_application of generation %d", gen)
		}
		area := make([]byte, 0x40)
		copy(area[0x20:0x30], n.key)
		if err := ecbEncrypt(kak, area); err != nil {
			return nil, false, 0, err
		}
		copy(header[0x300:0x340], area)
		copy(header[0x230:0x240], make([]byte, 16))
		header[0x207] = 0 // the key area key of an application
		standard = true
	}
	// installed from a file, never from a game card: marked as downloaded, as the eShop marks it
	header[0x204] = 0
	// its size, as it is
	binary.LittleEndian.PutUint64(header[0x208:], uint64(n.size))
	for i := range 4 {
		if _, _, ok := n.section(i); !ok {
			continue
		}
		sum := sha256.Sum256(header[0x400+i*0x200 : 0x600+i*0x200])
		if !bytes.Equal(sum[:], header[0x280+i*0x20:0x2A0+i*0x20]) {
			copy(header[0x280+i*0x20:], sum[:])
			fixed++
		}
	}
	if err := xts(k.header, header, true); err != nil {
		return nil, false, 0, err
	}
	return header, standard, fixed, nil
}

type progressReader struct {
	r    io.Reader
	step func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.step(int64(n))
	}
	return n, err
}

// CheckRebuilt opens a rebuilt NSP again from the disk and checks it: every NCA named by its
// SHA-256, with no rights ID and the right hash of each FS header; every content of each CNMT
// there with its size, and every NCA listed by a CNMT; then every NCA by its content ID and, where
// it can be told, its hash trees (switchfs.VerifyFile).
func CheckRebuilt(ctx context.Context, path string, progress func(string, int64, int64)) error {
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
	for _, name := range p.order {
		if !strings.HasSuffix(strings.ToLower(name), ".nca") {
			return fmt.Errorf("%s is not an NCA", name)
		}
	}
	g, err := readGame(k, []*pkgFile{p}, readCnmt)
	if err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, m := range g.metas {
		listed[strings.TrimSuffix(strings.ToLower(m.nca.name), ".cnmt.nca")] = true
		for _, c := range m.contents {
			n := g.ncas[c.id]
			if n == nil {
				return fmt.Errorf("%s.nca, listed by the CNMT of %016X, is not there", c.id, m.titleID)
			}
			listed[c.id] = true
		}
	}
	for id, n := range g.ncas {
		if !listed[id] {
			return fmt.Errorf("%s is not listed by any CNMT", n.name)
		}
		if n.hasRights() {
			return fmt.Errorf("%s still has a rights ID", n.name)
		}
		for i := range 4 {
			if _, _, ok := n.section(i); !ok {
				continue
			}
			sum := sha256.Sum256(n.fsHeader(i))
			if !bytes.Equal(sum[:], n.header[0x280+i*0x20:0x2A0+i*0x20]) {
				return fmt.Errorf("the hash of FS header %d of %s is wrong", i, n.name)
			}
		}
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
