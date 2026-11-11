package optimize

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// pkgFile is an NSP opened: its files by name.
type pkgFile struct {
	path    string
	f       *os.File
	entries map[string]pfsEntry
	order   []string
}

type pfsEntry struct {
	offset, size int64
}

func (p *pkgFile) ReadAt(b []byte, off int64) (int, error) { return p.f.ReadAt(b, off) }

func (p *pkgFile) Close() error { return p.f.Close() }

// openNsp opens an NSP (a PFS0): its file list.
func openNsp(path string) (*pkgFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	p := &pkgFile{path: path, f: f, entries: map[string]pfsEntry{}}
	fail := func(err error) (*pkgFile, error) {
		f.Close()
		return nil, err
	}
	head := make([]byte, 0x10)
	if _, err := f.ReadAt(head, 0); err != nil || string(head[:4]) != "PFS0" {
		return fail(fmt.Errorf("%s is not an NSP", filepath.Base(path)))
	}
	count := binary.LittleEndian.Uint32(head[4:])
	strSize := binary.LittleEndian.Uint32(head[8:])
	if count > 0x10000 || strSize > 1<<20 {
		return fail(fmt.Errorf("%s is not an NSP", filepath.Base(path)))
	}
	table := make([]byte, int(count)*0x18+int(strSize))
	if _, err := f.ReadAt(table, 0x10); err != nil {
		return fail(err)
	}
	strs := table[count*0x18:]
	data := int64(0x10) + int64(len(table))
	for i := range int(count) {
		e := table[i*0x18:]
		off, size := int64(binary.LittleEndian.Uint64(e)), int64(binary.LittleEndian.Uint64(e[8:]))
		at := binary.LittleEndian.Uint32(e[16:])
		if at >= strSize || off < 0 || size < 0 {
			return fail(fmt.Errorf("%s is damaged", filepath.Base(path)))
		}
		name := string(strs[at:])
		if z := strings.IndexByte(name, 0); z >= 0 {
			name = name[:z]
		}
		p.entries[name] = pfsEntry{offset: data + off, size: size}
		p.order = append(p.order, name)
	}
	return p, nil
}

func (p *pkgFile) read(name string) ([]byte, error) {
	e, ok := p.entries[name]
	if !ok {
		return nil, fmt.Errorf("%s has no %s", filepath.Base(p.path), name)
	}
	if e.size > 1<<20 {
		return nil, fmt.Errorf("%s: %s is too big", filepath.Base(p.path), name)
	}
	b := make([]byte, e.size)
	_, err := p.f.ReadAt(b, e.offset)
	return b, err
}

// asNsp gives an NSP of a game file: the file itself for an NSP, else one written into work
// (decompressed, or the secure partition of a game card), which the caller removes.
func asNsp(ctx context.Context, path string, work string, progress func(done, total int64)) (nsp string, temp []string, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	tmp := func(ext string) string {
		return filepath.Join(work, fmt.Sprintf(".nxpress-optimize-%d-%s%s", os.Getpid(), sanitize(base), ext))
	}
	switch ext {
	case ".nsp":
		return path, nil, nil
	case ".nsz":
		out := tmp(".nsp")
		if err := switchfs.DecompressNsz(ctx, path, out, progress); err != nil {
			os.Remove(out)
			return "", nil, err
		}
		return out, []string{out}, nil
	case ".xci":
		out := tmp(".nsp")
		if err := switchfs.ConvertXciToNsp(ctx, path, out, progress); err != nil {
			os.Remove(out)
			return "", nil, err
		}
		return out, []string{out}, nil
	case ".xcz":
		mid := tmp(".xci")
		if err := switchfs.DecompressNsz(ctx, path, mid, progress); err != nil {
			os.Remove(mid)
			return "", nil, err
		}
		defer os.Remove(mid)
		out := tmp(".nsp")
		if err := switchfs.ConvertXciToNsp(ctx, mid, out, progress); err != nil {
			os.Remove(out)
			return "", nil, err
		}
		return out, []string{out}, nil
	}
	return "", nil, fmt.Errorf("%s is not an NSP, NSZ, XCI or XCZ", filepath.Base(path))
}

func sanitize(s string) string {
	b := []rune{}
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b = append(b, r)
	}
	if len(b) > 60 {
		b = b[:60]
	}
	return string(b)
}

// ticket is a ticket of a package with its certificate.
type ticket struct {
	rights   string // lower case hex
	tik      []byte
	cert     []byte
	titleKey []byte // encrypted, of a common ticket
}

// readTickets reads the tickets of a package. Only common tickets give a title key: a
// personalized one (of an account) is kept out.
func readTickets(p *pkgFile) (map[string]*ticket, error) {
	out := map[string]*ticket{}
	for _, name := range p.order {
		if !strings.EqualFold(filepath.Ext(name), ".tik") {
			continue
		}
		tik, err := p.read(name)
		if err != nil {
			return nil, err
		}
		if len(tik) < 0x2C0 || binary.LittleEndian.Uint32(tik) != 0x10004 {
			continue // not a ticket signed with RSA-2048 and SHA-256, as the ones of games
		}
		if tik[0x281] != 0 {
			continue // personalized
		}
		t := &ticket{rights: hex.EncodeToString(tik[0x2A0:0x2B0]), tik: tik, titleKey: append([]byte(nil), tik[0x180:0x190]...)}
		certName := strings.TrimSuffix(name, filepath.Ext(name)) + ".cert"
		if cert, err := p.read(certName); err == nil {
			t.cert = cert
		}
		out[t.rights] = t
	}
	return out, nil
}

// game is what the merge takes of the packages given: the NCAs by content ID and the tickets.
type game struct {
	ncas    map[string]*nca // lower case content ID (the name without .nca / .cnmt.nca)
	tickets map[string]*ticket
	metas   []*meta
}

type meta struct {
	nca      *nca
	section  []byte
	titleID  uint64
	typ      uint8
	version  uint32
	contents []cnmtContent
}

// cnmtContent is a content a CNMT lists: its content ID (lower case), type and ID offset.
type cnmtContent struct {
	id       string
	typ      uint8
	idOffset uint8
}

// readCnmtFn reads the CNMT of a meta NCA (its header and section decrypted).
type readCnmtFn func(header, section []byte) (titleID uint64, typ uint8, version uint32, contents []cnmtContent, err error)

const (
	metaApplication = 0x80
	metaPatch       = 0x81
)

// readGame reads the NCAs, the tickets and the CNMTs of the packages.
func readGame(k *keyring, pkgs []*pkgFile, readCnmt readCnmtFn) (*game, error) {
	g := &game{ncas: map[string]*nca{}, tickets: map[string]*ticket{}}
	for _, p := range pkgs {
		t, err := readTickets(p)
		if err != nil {
			return nil, err
		}
		for r, x := range t {
			g.tickets[r] = x
		}
	}
	titleKeys := map[string][]byte{}
	for r, t := range g.tickets {
		titleKeys[r] = t.titleKey
	}
	for _, p := range pkgs {
		for _, name := range p.order {
			lower := strings.ToLower(name)
			if !strings.HasSuffix(lower, ".nca") {
				continue
			}
			e := p.entries[name]
			if e.size < 0xC00 {
				return nil, fmt.Errorf("%s: %s is not an NCA", filepath.Base(p.path), name)
			}
			n := &nca{name: name, file: p, offset: e.offset, size: e.size, header: make([]byte, 0xC00)}
			if _, err := p.f.ReadAt(n.header, e.offset); err != nil {
				return nil, err
			}
			if err := xts(k.header, n.header, false); err != nil {
				return nil, err
			}
			if string(n.header[0x200:0x204]) != "NCA3" {
				return nil, fmt.Errorf("%s: %s is not an NCA3 (wrong header_key, or damaged)", filepath.Base(p.path), name)
			}
			id := strings.TrimSuffix(strings.TrimSuffix(lower, ".nca"), ".cnmt")
			g.ncas[id] = n
			// a meta NCA: by its content type, or by its name as the tools that write NSP name it
			if n.contentType() != 1 && !strings.HasSuffix(lower, ".cnmt.nca") {
				continue
			}
			if err := n.unlock(k, titleKeys); err != nil {
				return nil, err
			}
			section, err := n.readSection(0)
			if err != nil {
				return nil, err
			}
			tid, typ, ver, contents, err := readCnmt(n.header, section)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			g.metas = append(g.metas, &meta{nca: n, section: section, titleID: tid, typ: typ, version: ver, contents: contents})
		}
	}
	return g, nil
}

// program is the NCA of the program a CNMT lists at an ID offset, and how many programs it lists.
func (g *game) program(m *meta, idOffset uint8) (*nca, int, error) {
	var found *nca
	programs := 0
	for _, c := range m.contents {
		if c.typ != 1 {
			continue
		}
		programs++
		if c.idOffset == idOffset {
			found = g.ncas[c.id]
			if found == nil {
				return nil, programs, fmt.Errorf("the program %s.nca of %016X v%d is not among the files given", c.id, m.titleID, m.version)
			}
		}
	}
	if found == nil {
		return nil, programs, fmt.Errorf("%016X v%d lists no program", m.titleID, m.version)
	}
	return found, programs, nil
}

// pair finds the game and its newest update among the CNMTs read.
func (g *game) pair() (*meta, *meta, error) {
	var base, update *meta
	for _, m := range g.metas {
		switch m.typ {
		case metaApplication:
			if base != nil && base.titleID != m.titleID {
				return nil, nil, errors.New("the files given hold more than one game: give one game and its update")
			}
			if base == nil || m.version > base.version {
				base = m
			}
		}
	}
	if base == nil {
		return nil, nil, errors.New("no base game among the files given")
	}
	for _, m := range g.metas {
		if m.typ == metaPatch && m.titleID == base.titleID|0x800 && (update == nil || m.version > update.version) {
			update = m
		}
	}
	if update == nil {
		return nil, nil, fmt.Errorf("no update of %016X among the files given", base.titleID)
	}
	return base, update, nil
}

// copyN copies n bytes of r from off to w.
func copyN(w io.Writer, r io.ReaderAt, off int64, n int64) error {
	_, err := io.Copy(w, io.NewSectionReader(r, off, n))
	return err
}
