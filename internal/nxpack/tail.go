package nxpack

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"sort"
	"strings"
)

var le = binary.LittleEndian

// layout is where the parts of the tail are, for the header.
type layout struct {
	archiveSize             uint64
	mediaOffset, mediaSize  uint64
	catalogOffset           uint64
	catalog                 []byte
	manifestOffset          uint64
	manifest                []byte
	dataSize                uint64
	blockOffset, blockCount uint64
	integrityOffset         uint64
	index, integrity        []byte
}

func (l *layout) header() []byte {
	h := make([]byte, headerSize)
	copy(h, "NXAR")
	le.PutUint16(h[6:], 2)
	le.PutUint32(h[8:], headerSize)
	le.PutUint16(h[12:], 1)
	le.PutUint64(h[16:], l.archiveSize)
	le.PutUint64(h[24:], l.catalogOffset)
	le.PutUint64(h[32:], uint64(len(l.catalog)))
	le.PutUint64(h[40:], l.mediaOffset)
	le.PutUint64(h[48:], l.mediaSize)
	if l.blockCount > 0 {
		le.PutUint64(h[56:], 2*slot)
		le.PutUint64(h[64:], l.dataSize)
		le.PutUint64(h[88:], l.blockOffset)
		le.PutUint64(h[104:], l.integrityOffset)
		le.PutUint64(h[112:], uint64(len(l.integrity)))
	}
	le.PutUint64(h[72:], l.manifestOffset)
	le.PutUint64(h[80:], uint64(len(l.manifest)))
	le.PutUint64(h[96:], l.blockCount)
	sum := sha256.Sum256(l.catalog)
	copy(h[120:], sum[:])
	sum = sha256.Sum256(l.manifest)
	copy(h[152:], sum[:])
	sum = sha256.Sum256(append(append([]byte{}, l.index...), l.integrity...))
	copy(h[184:], sum[:])
	le.PutUint32(h[224:], checkSize)
	le.PutUint32(h[252:], crc32.Checksum(h[:252], castagnoli))
	return h
}

func pad(b []byte, to int) []byte {
	for len(b)%to != 0 {
		b = append(b, 0)
	}
	return b
}

// tree is the folders of a version, each with its children by name.
type treeDir struct {
	path     string
	children []treeChild
	id       uint32
}

type treeChild struct {
	name string
	dir  *treeDir
	file int // the record of the file, when no dir
}

// placed is a version of a game, in the order of the catalog.
type placed struct {
	game    int
	version int
	program uint32
	root    *treeDir
	dirs    []*treeDir
	size    uint64
}

func contentID(v Snapshot) [16]byte {
	var id [16]byte
	if v.Kind == Dlc {
		le.PutUint64(id[:], v.Addon)
	}
	return id
}

func buildID(v Snapshot) [32]byte {
	var id [32]byte
	copy(id[:], v.BuildID)
	return id
}

func buildTail(games []Game, trees [][][]int, w *writer, dataEnd uint64) ([]byte, *layout, error) {
	order := make([]int, len(games))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := games[order[i]], games[order[j]]
		return a.Platform < b.Platform || a.Platform == b.Platform && a.ID < b.ID
	})

	// the versions in the order of the catalog: by program, then the DLC
	type programRow struct {
		id    uint64
		app   uint32
		first uint32
		count uint32
	}
	programs := []programRow{}
	snapshots := []placed{}
	type appRow struct {
		game                         int
		firstProgram, programCount   uint32
		firstSnapshot, snapshotCount uint32
		metadata                     int // index in snapshots, -1 when none
	}
	apps := []appRow{}
	for a, g := range order {
		game := games[g]
		row := appRow{game: g, firstProgram: uint32(len(programs)), firstSnapshot: uint32(len(snapshots)), metadata: -1}
		ids := []uint64{}
		for _, v := range game.Versions {
			if v.Kind != Dlc && !containsID(ids, v.Program) {
				ids = append(ids, v.Program)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		less := func(x, y Snapshot) bool {
			if x.Kind != y.Kind {
				return x.Kind < y.Kind
			}
			if x.Version != y.Version {
				return x.Version < y.Version
			}
			cx, cy := contentID(x), contentID(y)
			if c := bytes.Compare(cx[:], cy[:]); c != 0 {
				return c < 0
			}
			bx, by := buildID(x), buildID(y)
			return bytes.Compare(bx[:], by[:]) < 0
		}
		for _, id := range ids {
			program := programRow{id: id, app: uint32(a), first: uint32(len(snapshots))}
			indexes := []int{}
			for i, v := range game.Versions {
				if v.Kind != Dlc && v.Program == id {
					indexes = append(indexes, i)
				}
			}
			sort.Slice(indexes, func(i, j int) bool { return less(game.Versions[indexes[i]], game.Versions[indexes[j]]) })
			for _, i := range indexes {
				snapshots = append(snapshots, placed{game: g, version: i, program: uint32(len(programs))})
			}
			program.count = uint32(len(indexes))
			programs = append(programs, program)
		}
		dlc := []int{}
		for i, v := range game.Versions {
			if v.Kind == Dlc {
				dlc = append(dlc, i)
			}
		}
		sort.Slice(dlc, func(i, j int) bool { return less(game.Versions[dlc[i]], game.Versions[dlc[j]]) })
		for _, i := range dlc {
			snapshots = append(snapshots, placed{game: g, version: i, program: absent})
		}
		row.programCount = uint32(len(programs)) - row.firstProgram
		row.snapshotCount = uint32(len(snapshots)) - row.firstSnapshot
		for s := int(row.firstSnapshot); s < len(snapshots); s++ {
			v := game.Versions[snapshots[s].version]
			if v.Kind == Dlc || !hasMetadata(v) {
				continue
			}
			if row.metadata < 0 || v.Version >= game.Versions[snapshots[row.metadata].version].Version {
				row.metadata = s
			}
		}
		// two rows with the same sort key cannot be told apart
		for s := int(row.firstSnapshot) + 1; s < len(snapshots); s++ {
			x, y := game.Versions[snapshots[s-1].version], game.Versions[snapshots[s].version]
			if !less(x, y) && !less(y, x) && (x.Kind == Dlc) == (y.Kind == Dlc) && snapshots[s-1].program == snapshots[s].program {
				return nil, nil, errors.New("two versions of a game are the same kind and version")
			}
		}
		apps = append(apps, row)
	}

	// the trees of the versions: their folders, each with an ID, and their files
	dirs := []*treeDir{}
	for s := range snapshots {
		p := &snapshots[s]
		v := games[p.game].Versions[p.version]
		root := &treeDir{}
		byPath := map[string]*treeDir{"": root}
		var dirOf func(path string) *treeDir
		dirOf = func(path string) *treeDir {
			if d, ok := byPath[path]; ok {
				return d
			}
			parent, name := "", path
			if i := strings.LastIndexByte(path, '/'); i >= 0 {
				parent, name = path[:i], path[i+1:]
			}
			d := &treeDir{path: path}
			byPath[path] = d
			up := dirOf(parent)
			up.children = append(up.children, treeChild{name: name, dir: d})
			return d
		}
		for _, d := range v.Dirs {
			dirOf(d)
		}
		files := append([]File(nil), v.Files...)
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		for i, f := range files {
			parent, name := "", f.Path
			if k := strings.LastIndexByte(f.Path, '/'); k >= 0 {
				parent, name = f.Path[:k], f.Path[k+1:]
			}
			if _, clash := byPath[f.Path]; clash {
				return nil, nil, errors.New("a path of a version is a file and a folder")
			}
			d := dirOf(parent)
			d.children = append(d.children, treeChild{name: name, file: trees[p.game][p.version][i]})
			p.size += uint64(f.Size)
		}
		var number func(d *treeDir)
		number = func(d *treeDir) {
			sort.Slice(d.children, func(i, j int) bool { return d.children[i].name < d.children[j].name })
			d.id = uint32(len(dirs))
			dirs = append(dirs, d)
			p.dirs = append(p.dirs, d)
			for _, c := range d.children {
				if c.dir != nil {
					number(c.dir)
				}
			}
		}
		number(root)
		p.root = root
	}

	// the manifest
	entryCount := 0
	for _, d := range dirs {
		entryCount += len(d.children)
	}
	pool := []byte{}
	names := map[string]uint64{}
	nameAt := func(name string) uint64 {
		if at, ok := names[name]; ok {
			return at
		}
		at := uint64(len(pool))
		names[name] = at
		pool = append(pool, name...)
		return at
	}
	for _, d := range dirs {
		for _, c := range d.children {
			nameAt(c.name)
		}
	}
	dirTable := uint64(manifestHead)
	fileTable := uint64(0)
	next := dirTable + uint64(len(dirs))*dirSize
	if len(w.files) > 0 {
		fileTable = align(next, 8)
		next = fileTable + uint64(len(w.files))*fileSize
	}
	entryTable := uint64(0)
	if entryCount > 0 {
		entryTable = align(next, 8)
		next = entryTable + uint64(entryCount)*entrySize
	}
	refTable := uint64(0)
	if len(w.refs) > 0 {
		refTable = align(next, 4)
		next = refTable + uint64(len(w.refs))*4
	}
	poolOffset := align(next, 8)
	manifest := make([]byte, poolOffset)
	manifest = pad(append(manifest, pool...), 8)
	copy(manifest, "NXMF")
	le.PutUint16(manifest[4:], 1)
	le.PutUint16(manifest[6:], manifestHead)
	le.PutUint64(manifest[8:], uint64(len(manifest)))
	le.PutUint32(manifest[16:], uint32(len(dirs)))
	le.PutUint32(manifest[20:], uint32(len(w.files)))
	le.PutUint32(manifest[24:], uint32(entryCount))
	le.PutUint32(manifest[28:], uint32(len(w.refs)))
	le.PutUint64(manifest[32:], dirTable)
	le.PutUint64(manifest[40:], fileTable)
	le.PutUint64(manifest[48:], entryTable)
	le.PutUint64(manifest[56:], refTable)
	le.PutUint64(manifest[64:], poolOffset)
	le.PutUint64(manifest[72:], uint64(len(pool)))
	entry := uint32(0)
	for i, d := range dirs {
		row := manifest[dirTable+uint64(i)*dirSize:]
		le.PutUint32(row, entry)
		le.PutUint32(row[4:], uint32(len(d.children)))
		for _, c := range d.children {
			e := manifest[entryTable+uint64(entry)*entrySize:]
			le.PutUint64(e, poolOffset+names[c.name])
			le.PutUint32(e[8:], uint32(len(c.name)))
			if c.dir != nil {
				le.PutUint32(e[12:], c.dir.id)
				e[16] = 1
			} else {
				le.PutUint32(e[12:], uint32(c.file))
				e[16] = 2
			}
			entry++
		}
	}
	for i, f := range w.files {
		row := manifest[fileTable+uint64(i)*fileSize:]
		le.PutUint64(row, f.size)
		le.PutUint32(row[8:], f.first)
		le.PutUint32(row[12:], f.count)
		row[16] = f.log2
		copy(row[20:], f.sha[:])
	}
	for i, id := range w.refs {
		le.PutUint32(manifest[refTable+uint64(i)*4:], id)
	}

	// the media: the NACP of each game and its icon
	type mediaItem struct {
		kind uint16
		data []byte
	}
	media := []mediaItem{}
	addMedia := func(kind uint16, data []byte) uint32 {
		if len(data) == 0 {
			return absent
		}
		media = append(media, mediaItem{kind, data})
		return uint32(len(media) - 1)
	}
	type appMedia struct{ icon, nacp uint32 }
	appsMedia := make([]appMedia, len(apps))
	for a, row := range apps {
		appsMedia[a] = appMedia{absent, absent}
		if row.metadata < 0 {
			continue
		}
		v := games[row.game].Versions[snapshots[row.metadata].version]
		// only a JPEG or a PNG is an icon
		switch {
		case bytes.HasPrefix(v.Icon, []byte("\x89PNG\r\n\x1a\n")):
			appsMedia[a].icon = addMedia(1, v.Icon)
		case bytes.HasPrefix(v.Icon, []byte{0xFF, 0xD8, 0xFF}):
			appsMedia[a].icon = addMedia(2, v.Icon)
		}
		appsMedia[a].nacp = addMedia(4, v.Nacp)
	}
	var mediaRegion []byte
	if len(media) > 0 {
		mediaRegion = pad(make([]byte, mediaHeader+len(media)*mediaRecord), 16)
		payloadArea := len(mediaRegion)
		for i, m := range media {
			mediaRegion = pad(mediaRegion, 16)
			row := mediaRegion[mediaHeader+i*mediaRecord:]
			le.PutUint16(row, m.kind)
			le.PutUint64(row[4:], uint64(len(m.data)))
			le.PutUint64(row[12:], uint64(len(m.data)))
			le.PutUint64(row[20:], uint64(len(mediaRegion)))
			sum := sha256.Sum256(m.data)
			copy(row[28:], sum[:])
			mediaRegion = append(mediaRegion, m.data...)
		}
		copy(mediaRegion, "NXMD")
		le.PutUint16(mediaRegion[4:], 1)
		le.PutUint16(mediaRegion[6:], mediaHeader)
		le.PutUint64(mediaRegion[8:], uint64(len(mediaRegion)))
		le.PutUint32(mediaRegion[16:], uint32(len(media)))
		le.PutUint16(mediaRegion[20:], mediaRecord)
		le.PutUint32(mediaRegion[24:], mediaHeader)
		le.PutUint32(mediaRegion[28:], uint32(payloadArea))
	}

	// the catalog
	strs := []byte{}
	strAt := map[string]uint32{}
	str := func(s string) (uint32, uint32) {
		if s == "" {
			return absent, 0
		}
		if at, ok := strAt[s]; ok {
			return at, uint32(len(s))
		}
		at := uint32(len(strs))
		strAt[s] = at
		strs = append(strs, s...)
		return at, uint32(len(s))
	}
	type localized struct{ tag, name string }
	locals := [][]localized{}
	localizedCount := 0
	for _, row := range apps {
		list := []localized{}
		if row.metadata >= 0 {
			v := games[row.game].Versions[snapshots[row.metadata].version]
			for tag, name := range v.Names {
				if validTag(tag) && name != "" && utf8Text(name) {
					list = append(list, localized{tag, name})
				}
			}
			sort.Slice(list, func(i, j int) bool { return list[i].tag < list[j].tag })
		}
		locals = append(locals, list)
		localizedCount += len(list)
	}
	appTable := uint64(catalogHeader)
	programTable := uint64(0)
	next = appTable + uint64(len(apps))*appSize
	if len(programs) > 0 {
		programTable = next
		next += uint64(len(programs)) * programSize
	}
	snapshotTable := next
	next += uint64(len(snapshots)) * snapshotSize
	localizedTable := uint64(0)
	if localizedCount > 0 {
		localizedTable = next
		next += uint64(localizedCount) * localizedSize
	}
	strPool := align(next, 8)
	catalog := make([]byte, strPool)
	localizedAt := uint32(0)
	for a, row := range apps {
		game := games[row.game]
		r := catalog[appTable+uint64(a)*appSize:]
		le.PutUint64(r, game.ID)
		r[8] = game.Platform
		flags := byte(0)
		for _, v := range game.Versions {
			if v.Kind == Base {
				flags |= 1
			}
			if v.Kind == Dlc {
				flags |= 2
			}
		}
		r[9] = flags
		if row.programCount > 0 {
			le.PutUint32(r[12:], row.firstProgram)
		} else {
			le.PutUint32(r[12:], absent)
		}
		le.PutUint32(r[16:], row.programCount)
		le.PutUint32(r[20:], row.firstSnapshot)
		le.PutUint32(r[24:], row.snapshotCount)
		title, publisher := "", ""
		if row.metadata >= 0 {
			v := game.Versions[snapshots[row.metadata].version]
			title, publisher = v.Title, v.Publisher
		}
		o, l := str(title)
		le.PutUint32(r[28:], o)
		le.PutUint32(r[32:], l)
		o, l = str(publisher)
		le.PutUint32(r[36:], o)
		le.PutUint32(r[40:], l)
		le.PutUint32(r[44:], appsMedia[a].icon)
		le.PutUint32(r[48:], localizedAt)
		le.PutUint32(r[52:], uint32(len(locals[a])))
		le.PutUint32(r[56:], appsMedia[a].nacp)
		hasMeta := title != "" || publisher != "" || appsMedia[a].icon != absent || appsMedia[a].nacp != absent || len(locals[a]) > 0
		if row.metadata >= 0 && hasMeta {
			le.PutUint32(r[68:], uint32(row.metadata))
		} else {
			le.PutUint32(r[68:], absent)
		}
		for _, loc := range locals[a] {
			e := catalog[localizedTable+uint64(localizedAt)*localizedSize:]
			o, l := str(loc.tag)
			le.PutUint32(e, o)
			le.PutUint32(e[4:], l)
			o, l = str(loc.name)
			le.PutUint32(e[8:], o)
			le.PutUint32(e[12:], l)
			le.PutUint32(e[16:], absent)
			localizedAt++
		}
	}
	for i, p := range programs {
		r := catalog[programTable+uint64(i)*programSize:]
		le.PutUint64(r, p.id)
		le.PutUint32(r[8:], p.app)
		le.PutUint32(r[32:], p.first)
		le.PutUint32(r[36:], p.count)
		le.PutUint32(r[40:], absent)
	}
	identities := map[[32]byte]bool{}
	for s, p := range snapshots {
		game := games[p.game]
		v := game.Versions[p.version]
		r := catalog[snapshotTable+uint64(s)*snapshotSize:]
		le.PutUint32(r, p.program)
		r[4] = v.Kind
		bid := buildID(v)
		if bid != [32]byte{} {
			r[5] |= 1
		}
		if v.Kind == Dlc {
			r[5] |= 2
		}
		le.PutUint32(r[8:], v.Version)
		le.PutUint32(r[12:], p.root.id)
		le.PutUint64(r[16:], p.size)
		o, l := str(v.Display)
		le.PutUint32(r[24:], o)
		le.PutUint32(r[28:], l)
		cid := contentID(v)
		copy(r[32:], cid[:])
		identity := snapshotIdentity(game, v, p, w)
		if identities[identity] {
			return nil, nil, errors.New("two versions of a game hold the same files with the same kind and version")
		}
		identities[identity] = true
		copy(r[56:], identity[:])
		copy(r[88:], bid[:])
	}
	catalog = pad(append(catalog, strs...), 8)
	copy(catalog, "NXCT")
	le.PutUint16(catalog[4:], 1)
	le.PutUint16(catalog[6:], catalogHeader)
	le.PutUint64(catalog[8:], uint64(len(catalog)))
	le.PutUint32(catalog[20:], uint32(len(apps)))
	le.PutUint32(catalog[24:], uint32(len(programs)))
	le.PutUint32(catalog[28:], uint32(len(snapshots)))
	le.PutUint32(catalog[32:], uint32(localizedCount))
	le.PutUint64(catalog[48:], appTable)
	le.PutUint64(catalog[56:], programTable)
	le.PutUint64(catalog[64:], snapshotTable)
	le.PutUint64(catalog[72:], localizedTable)
	le.PutUint64(catalog[96:], strPool)
	le.PutUint64(catalog[104:], uint64(len(strs)))
	if len(catalog) > 16<<20 {
		return nil, nil, errors.New("the catalog would be larger than the format allows")
	}

	// the index of the blocks and the table of checks
	index := make([]byte, len(w.blocks)*blockSize)
	for i, b := range w.blocks {
		r := index[i*blockSize:]
		le.PutUint64(r, b.offset)
		le.PutUint32(r[8:], b.stored)
		le.PutUint32(r[12:], b.logical)
		r[16] = b.codec
		le.PutUint32(r[20:], b.firstCheck)
	}
	integrity := make([]byte, len(w.checks)*4)
	for i, c := range w.checks {
		le.PutUint32(integrity[i*4:], c)
	}

	// the tail, each part 16-byte aligned after the one before
	l := &layout{blockCount: uint64(len(w.blocks)), index: index, integrity: integrity, catalog: catalog, manifest: manifest}
	if len(w.blocks) > 0 {
		l.dataSize = dataEnd - 2*slot
	}
	start := align(dataEnd, 16)
	tail := []byte{}
	place := func(part []byte) uint64 {
		at := start + uint64(len(pad(tail, 16)))
		tail = append(pad(tail, 16), part...)
		return at
	}
	if len(mediaRegion) > 0 {
		l.mediaOffset, l.mediaSize = place(mediaRegion), uint64(len(mediaRegion))
	}
	l.catalogOffset = place(catalog)
	l.manifestOffset = place(manifest)
	l.archiveSize = l.manifestOffset + uint64(len(manifest))
	if len(w.blocks) > 0 {
		l.blockOffset = place(index)
		l.integrityOffset = place(integrity)
		l.archiveSize = l.integrityOffset + uint64(len(integrity))
	}
	return tail, l, nil
}

func containsID(ids []uint64, id uint64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func hasMetadata(v Snapshot) bool {
	return v.Title != "" || v.Publisher != "" || len(v.Icon) > 0 || len(v.Nacp) > 0 || len(v.Names) > 0
}

func validTag(tag string) bool {
	if tag == "" || len(tag) > 63 {
		return false
	}
	for _, part := range strings.Split(tag, "-") {
		if part == "" || len(part) > 8 {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				return false
			}
		}
	}
	return true
}

func utf8Text(s string) bool {
	return strings.IndexByte(s, 0) < 0 && len(s) <= 4096
}

// snapshotIdentity is the identity of a version the format defines: its scope, kind, version and
// content ID, then every folder and file of its tree by path, the files with their size and
// SHA-256.
func snapshotIdentity(game Game, v Snapshot, p placed, w *writer) [32]byte {
	h := sha256.New()
	h.Write([]byte("NX-SNAPSHOT-ID\x00"))
	var b [8]byte
	le.PutUint16(b[:], 3)
	h.Write(b[:2])
	h.Write([]byte{game.Platform})
	le.PutUint64(b[:], game.ID)
	h.Write(b[:])
	program := v.Program
	if v.Kind == Dlc {
		program = 0xffffffffffffffff
	}
	le.PutUint64(b[:], program)
	h.Write(b[:])
	h.Write([]byte{v.Kind})
	le.PutUint32(b[:], v.Version)
	h.Write(b[:4])
	cid := contentID(v)
	h.Write(cid[:])
	type node struct {
		path string
		file int // -1 for a folder
	}
	nodes := []node{}
	var walk func(d *treeDir, prefix string)
	walk = func(d *treeDir, prefix string) {
		nodes = append(nodes, node{prefix, -1})
		for _, c := range d.children {
			path := c.name
			if prefix != "" {
				path = prefix + "/" + c.name
			}
			if c.dir != nil {
				walk(c.dir, path)
			} else {
				nodes = append(nodes, node{path, c.file})
			}
		}
	}
	walk(p.root, "")
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].path < nodes[j].path })
	le.PutUint32(b[:], uint32(len(nodes)))
	h.Write(b[:4])
	for _, n := range nodes {
		if n.file < 0 {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{2})
		}
		le.PutUint32(b[:], uint32(len(n.path)))
		h.Write(b[:4])
		h.Write([]byte(n.path))
		if n.file >= 0 {
			f := w.files[n.file]
			le.PutUint64(b[:], f.size)
			h.Write(b[:])
			h.Write(f.sha[:])
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
