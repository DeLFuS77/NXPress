package nxpack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/nup"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A dump folder as NXPress packs it into a NUP (base, updates/<version>, dlc/<title ID>/<index>)
// becomes a .nx as the reference packer makes one of a NUP: each version a tree of exefs, romfs
// and data, with the control data of the folder (control, logo, legal, meta) in data/<name>;
// each update complete, its ExeFS in place of the one of the game and its other files over the
// ones of the game. Unpacking a .nx gives such a folder back.

// the folders of a package that go into data/
var dataFolders = map[string]bool{"control": true, "logo": true, "legal": true, "meta": true}

// what the folders of a dump leave out: the listings an FTP client leaves
const listing = ".listing"

// the languages of the names of a NACP, in its order, by their tags
var nacpLanguages = []string{"en-us", "en-gb", "ja", "fr", "de", "es-419", "es", "it", "nl", "fr-ca", "pt", "ru", "ko",
	"zh-hant", "zh-hans", "pt-br", "pl", "th"}

// FromDump reads a dump folder into the game a .nx is written of.
func FromDump(root string) (Game, []string, error) {
	sources, notes, err := nup.Discover(root)
	if err != nil {
		return Game{}, notes, err
	}
	game := Game{}
	var baseTree map[string]File
	var baseDirs []string
	platforms := map[byte]bool{}
	for _, source := range sources {
		tree, dirs, err := packageTree(source.Dir)
		if err != nil {
			return Game{}, notes, err
		}
		v := Snapshot{Version: source.Version}
		if len(source.BuildID) == 32 {
			v.BuildID = append([]byte(nil), source.BuildID...)
		}
		switch source.Kind {
		case nup.KindBase:
			v.Kind, v.Program = Base, source.TitleID
			baseTree, baseDirs = tree, dirs
			game.ID = source.TitleID
		case nup.KindUpdate:
			v.Kind, v.Program = Update, source.TitleID&^0x800
			if source.Flags&1 != 0 || baseTree == nil {
				v.Kind = StandaloneUpdate
			} else {
				tree, dirs = overlay(baseTree, baseDirs, tree, dirs)
			}
			if game.ID == 0 {
				game.ID = v.Program
			}
		case nup.KindAddon:
			v.Kind, v.Addon = Dlc, source.TitleID
			if game.ID == 0 && source.TitleID&0xFFF != 0 {
				game.ID = (source.TitleID &^ 0xFFF) - 0x1000
			}
		default:
			continue
		}
		if v.Kind != Dlc {
			if v.Program == 0 {
				return Game{}, notes, fmt.Errorf("%s: no main.npdm tells its program", source.Dir)
			}
			if platform := npdmPlatform(source.Npdm); platform != 0 {
				platforms[platform] = true
			}
			if source.Control != "" {
				control(&v, source.Control)
			}
		}
		for _, f := range tree {
			v.Files = append(v.Files, f)
		}
		sort.Slice(v.Files, func(i, j int) bool { return v.Files[i].Path < v.Files[j].Path })
		v.Dirs = dirs
		game.Versions = append(game.Versions, v)
	}
	if game.ID == 0 {
		return Game{}, notes, errors.New("the folder does not tell the ID of its game")
	}
	switch len(platforms) {
	case 1:
		for platform := range platforms {
			game.Platform = platform
		}
	case 2:
		return Game{}, notes, errors.New("the folder holds programs of the Switch and of the Switch 2, which this version does not pack into one .nx")
	}
	for _, v := range game.Versions {
		if v.Kind == Dlc && v.Addon&0xFFF != 0 && (v.Addon&^0xFFF)-0x1000 != game.ID {
			return Game{}, notes, fmt.Errorf("the DLC %016X is of another game than %016X", v.Addon, game.ID)
		}
	}
	return game, notes, nil
}

// npdmPlatform is the console of a program by its main.npdm: its ACID 0x200 bytes into it for
// the Switch, 0x300 for the Switch 2; 0 when it does not tell.
func npdmPlatform(npdm []byte) byte {
	if len(npdm) < 0x80 || string(npdm[:4]) != "META" {
		return 0
	}
	acid := int(le.Uint32(npdm[0x78:]))
	for _, c := range []struct {
		at       int
		platform byte
	}{{0x200, 1}, {0x300, 2}} {
		if acid >= 0 && acid+c.at+4 <= len(npdm) && string(npdm[acid+c.at:acid+c.at+4]) == "ACID" {
			return c.platform
		}
	}
	return 0
}

// control gives a version the NACP, the icon, the names and the version its control folder holds.
func control(v *Snapshot, dir string) {
	nacp, err := os.ReadFile(filepath.Join(dir, "control.nacp"))
	if err != nil || len(nacp) < 0x3080 {
		return
	}
	v.Nacp = nacp
	if icon := nup.ControlIcon(dir); len(icon) > 0 {
		v.Icon = icon
	}
	if v.Display == "" {
		v.Display = cText(nacp[0x3060:0x3070])
	}
	entries, count, err := switchfs.NacpTitleEntries(nacp)
	if err != nil {
		return
	}
	v.Names = map[string]string{}
	for i := 0; i < count && i < len(nacpLanguages); i++ {
		entry := entries[i*0x300:]
		name := cText(entry[:0x200])
		if name == "" {
			continue
		}
		v.Names[nacpLanguages[i]] = name
		if v.Title == "" || i == 0 {
			v.Title, v.Publisher = name, cText(entry[0x200:0x300])
		}
	}
}

// cText is the text of a field of a NACP: up to its first zero byte.
func cText(field []byte) string {
	if i := bytes.IndexByte(field, 0); i >= 0 {
		field = field[:i]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(field), ""))
}

// packageTree is the tree of a package as a version of a .nx holds it: its exefs, romfs and data,
// and its control folders in data/.
func packageTree(dir string) (map[string]File, []string, error) {
	tree := map[string]File{}
	dirs := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == listing {
			continue
		}
		if !entry.IsDir() {
			return nil, nil, fmt.Errorf("%s: the file %s is not in exefs, romfs or data", dir, name)
		}
		logical := name
		switch {
		case name == "exefs" || name == "romfs" || name == "data":
		case dataFolders[name]:
			logical = "data/" + name
			if !containsDir(dirs, "data") {
				dirs = append(dirs, "data")
			}
		case name == "program":
			// the layout of OunceCake: nup.Discover took program/<index> as the folder
			continue
		default:
			return nil, nil, fmt.Errorf("%s: the folder %s is not exefs, romfs, data or control data", dir, name)
		}
		base := filepath.Join(dir, name)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == listing {
				return nil
			}
			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			inside := logical
			if rel != "." {
				inside = logical + "/" + filepath.ToSlash(rel)
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("%s is a link: the folder is packed as it is, links left out", path)
			}
			if d.IsDir() {
				dirs = append(dirs, inside)
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			tree[inside] = File{Path: inside, Source: path, Size: info.Size()}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Strings(dirs)
	return tree, dirs, nil
}

func containsDir(dirs []string, dir string) bool {
	for _, d := range dirs {
		if d == dir {
			return true
		}
	}
	return false
}

// overlay is the complete tree of an update: the files of the game but its ExeFS, which the one of
// the update replaces whole, with the files of the update over them.
func overlay(base map[string]File, baseDirs []string, update map[string]File, updateDirs []string) (map[string]File, []string) {
	replaced := containsDir(updateDirs, "exefs")
	inExefs := func(path string) bool { return replaced && (path == "exefs" || strings.HasPrefix(path, "exefs/")) }
	tree := map[string]File{}
	for path, f := range base {
		if !inExefs(path) {
			tree[path] = f
		}
	}
	for path, f := range update {
		tree[path] = f
	}
	seen := map[string]bool{}
	for _, d := range baseDirs {
		if !inExefs(d) {
			seen[d] = true
		}
	}
	for _, d := range updateDirs {
		seen[d] = true
	}
	// a folder of the game where the update has a file is the file of the update
	dirs := []string{}
	for d := range seen {
		if _, file := tree[d]; !file {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return tree, dirs
}

// ToDump writes the versions of a .nx into dest, a folder that must not be there yet, as a dump
// folder NXPress packs into a NUP: the game in base, each update in updates/<version>, each DLC in
// dlc/<title ID>/<index>, with data/control and the others in control and the others again.
// Every file is checked as it is written. progress is told the bytes written of the total.
func ToDump(ctx context.Context, path string, dest string, progress func(done int64, total int64)) (err error) {
	data, err := switchfs.OpenNxData(path)
	if err != nil {
		return err
	}
	defer data.Close()
	if len(data.Applications) != 1 {
		return fmt.Errorf("the .nx holds %d games: a dump folder holds one", len(data.Applications))
	}
	app := data.Applications[0]
	type job struct {
		dest  string
		index int
	}
	jobs := []job{}
	folders := []string{}
	total := int64(0)
	dlcIndex := map[uint64]int{}
	bases := 0
	for s, snapshot := range app.Snapshots {
		var folder string
		switch snapshot.Type {
		case switchfs.NxSnapshotBase:
			bases++
			folder = "base"
		case switchfs.NxSnapshotUpdate, switchfs.NxSnapshotStandaloneUpdate:
			folder = filepath.Join("updates", strconv.FormatUint(uint64(snapshot.Version), 10))
		case switchfs.NxSnapshotDlc:
			if snapshot.AddonTitleId == 0 {
				return errors.New("a DLC of the .nx keeps its title ID in its CNMT (it was packed from an NSP): this version does not unpack it")
			}
			folder = filepath.Join("dlc", fmt.Sprintf("%016x", snapshot.AddonTitleId), strconv.Itoa(dlcIndex[snapshot.AddonTitleId]))
			dlcIndex[snapshot.AddonTitleId]++
		default:
			return errors.New("the .nx holds a version of an unknown kind")
		}
		if bases > 1 {
			return errors.New("the .nx holds the game in more than one program: a dump folder holds one")
		}
		root := filepath.Join(dest, folder)
		folders = append(folders, root)
		err := data.Walk(0, s, func(inside string) error {
			// data/ alone, which only holds the control folders, is not a folder of the dump
			if inside != "" && inside != "data" {
				folders = append(folders, filepath.Join(root, dumpPath(inside)))
			}
			return nil
		}, func(inside string, index int) error {
			total += int64(data.File(index).Size)
			jobs = append(jobs, job{filepath.Join(root, dumpPath(inside)), index})
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dest)
		}
	}()
	for _, folder := range folders {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return err
		}
	}
	done := int64(0)
	for _, j := range jobs {
		if err := os.MkdirAll(filepath.Dir(j.dest), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(j.dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		err = data.WriteFile(ctx, j.index, out, func(n int64) {
			done += n
			if progress != nil {
				progress(done, total)
			}
		})
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", j.dest, err)
		}
	}
	return nil
}

// dumpPath is where a path of a version goes in its folder: data/control is control again.
func dumpPath(inside string) string {
	if rest, ok := strings.CutPrefix(inside, "data/"); ok {
		name, _, _ := strings.Cut(rest, "/")
		if dataFolders[name] {
			inside = rest
		}
	}
	return filepath.FromSlash(inside)
}
