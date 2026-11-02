package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// pack merges the files given into one multi-title file (a SuperNSP), checked as it is
// written; the delta fragments of the updates are left out, as installers do not need them.
func pack(ctx context.Context, o options, out output) error {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	if len(missing) > 0 {
		return fmt.Errorf("not found: %s", strings.Join(missing, ", "))
	}
	// the file chosen for the pack is never one of its files: it would be written over while it
	// is read, then deleted. Given again when it is a pack made before of the others (the window
	// keeps the result in its list), it is left out; else it holds what the pack would lose
	chosen, err := filepath.Abs(o.pack)
	if err != nil {
		return err
	}
	given := sources
	sources = nil
	for _, source := range given {
		if source != chosen && !sameFile(source, chosen) {
			sources = append(sources, source)
		}
	}
	if len(sources) < len(given) && !coveredBy(chosen, sources) {
		return fmt.Errorf("the pack would take the name of %s, one of the files to combine", filepath.Base(chosen))
	}
	if len(sources) < 2 {
		return errors.New("a pack needs at least two files")
	}
	// the name is known before anything is written, where it was chosen (the output folder is
	// for the new files of the other actions): taken, the work does not start
	extension, err := packKind(sources)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(chosen, filepath.Ext(chosen)) + extension
	for _, source := range sources {
		if sameFile(source, name) {
			return fmt.Errorf("the pack would take the name of %s, one of the files to combine", filepath.Base(source))
		}
	}
	if err := free(o, name); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	// written in its folder, so its name is a rename on the same drive
	written := filepath.Join(filepath.Dir(name), fmt.Sprintf(".nxpress-pack-%d%s", time.Now().UnixNano(), extension))
	defer os.Remove(written)
	note, check, err := writePack(ctx, sources, written, o.pack, out)
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return err
	}
	kept := incompleteNote(o, check, name, sources)
	out.done(o.pack, name, joinNotes(keptNote(o, check, note), kept), nil)
	if kept != "" {
		return nil
	}
	for _, source := range sources {
		if _, _, err := removeSource(o, source, check, name, ""); err != nil {
			return err
		}
	}
	return nil
}

// coveredBy tells whether the others hold every title of a file, in its version or a newer one:
// a pack made of them before.
func coveredBy(file string, others []string) bool {
	titles, err := readTitles(file)
	if err != nil {
		return false
	}
	newest := map[string]int{}
	for _, other := range others {
		read, err := readTitles(other)
		if err != nil {
			return false
		}
		for _, title := range read {
			id := strings.ToLower(title.TitleId)
			if version, ok := newest[id]; !ok || title.Version > version {
				newest[id] = title.Version
			}
		}
	}
	for _, title := range titles {
		if version, ok := newest[strings.ToLower(title.TitleId)]; !ok || version < title.Version {
			return false
		}
	}
	return true
}

// incompleteNote is why the originals of a pack stay with --rm-source although it is checked:
// it lacks some of the titles it keeps. "" when it holds them whole, or nothing is deleted.
func incompleteNote(o options, check switchfs.FileCheck, pack string, sources []string) string {
	if !o.rmSource || check.Partial {
		return ""
	}
	lacks, err := packLacks(pack, sources)
	switch {
	case err != nil:
		return "the originals are kept: the pack cannot be read again: " + printable(err.Error())
	case len(lacks) > 0:
		return "the originals are kept: the pack lacks " + strings.Join(lacks, ", ")
	}
	return ""
}

func joinNotes(notes ...string) string {
	kept := []string{}
	for _, note := range notes {
		if note != "" {
			kept = append(kept, note)
		}
	}
	return strings.Join(kept, "; ")
}

// packLacks tells what a pack lacks of the titles it keeps: of each, the NCA files its CNMT
// names (but its delta fragments, left out on purpose), and a ticket when an original has one
// for it. The check of a pack compares each file it holds with the one it comes from, not
// what should be there: a file merged in and lost on the way (the ticket of an update kept,
// when a SuperNSP holds two versions of it under one name) would go with its originals.
func packLacks(pack string, sources []string) ([]string, error) {
	files, err := packageFiles(pack)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	tickets := map[string]bool{}
	for _, file := range files {
		name := strings.ToLower(file)
		held[name] = true
		// a ticket is named by its rights ID, the title ID first
		if strings.HasSuffix(name, ".tik") && len(name) > 16 {
			tickets[name[:16]] = true
		}
	}
	wanted := map[string]bool{}
	for _, source := range sources {
		files, err := packageFiles(source)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if name := strings.ToLower(file); strings.HasSuffix(name, ".tik") && len(name) > 16 {
				wanted[name[:16]] = true
			}
		}
	}
	titles, err := readTitles(pack)
	if err != nil {
		return nil, err
	}
	sort.Slice(titles, func(i, j int) bool { return titles[i].TitleId < titles[j].TitleId })
	lacks := []string{}
	for _, title := range titles {
		id := strings.ToLower(title.TitleId)
		for _, kind := range contentOrder {
			content, ok := title.Contents[kind]
			if !ok || content.ID == "" || kind == "DeltaFragment" {
				continue
			}
			nca := strings.ToLower(content.ID)
			if !held[nca+".nca"] && !held[nca+".ncz"] && !held[nca+".cnmt.nca"] && !held[nca+".cnmt.ncz"] {
				lacks = append(lacks, fmt.Sprintf("the %s NCA of %s v%d", kind, strings.ToUpper(id), title.Version))
			}
		}
		if wanted[id] && !tickets[id] {
			lacks = append(lacks, "the ticket of "+strings.ToUpper(id))
		}
	}
	return lacks, nil
}

// packageFiles are the names of the files a game file holds: in its PFS0, or in the secure
// partition of a game card.
func packageFiles(path string) ([]string, error) {
	if extension := strings.ToLower(filepath.Ext(path)); extension != ".xci" && extension != ".xcz" {
		pfs0, err := switchfs.ReadPfs0File(path)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(pfs0.Files))
		for i, file := range pfs0.Files {
			names[i] = file.Name
		}
		return names, nil
	}
	reader, err := switchfs.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	header := make([]byte, 0x200)
	if err := readAt(reader, header, 0); err != nil {
		return nil, err
	}
	if string(header[0x100:0x104]) != "HEAD" {
		return nil, errors.New("it is not a game card")
	}
	root := int64(binary.LittleEndian.Uint64(header[0x130:0x138]))
	partitions, err := hfs0Files(reader, root)
	if err != nil {
		return nil, err
	}
	for _, partition := range partitions {
		if partition.name != "secure" {
			continue
		}
		files, err := hfs0Files(reader, root+partition.offset)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(files))
		for i, file := range files {
			names[i] = file.name
		}
		return names, nil
	}
	return nil, errors.New("the game card has no secure partition")
}

// hfs0File is a file of an HFS0, the file system of a game card, and where it starts.
type hfs0File struct {
	name   string
	offset int64
}

// hfs0Files reads the table of the HFS0 at offset: its files, each from the start of the HFS0.
func hfs0Files(reader io.ReaderAt, offset int64) ([]hfs0File, error) {
	head := make([]byte, 0x10)
	if err := readAt(reader, head, offset); err != nil {
		return nil, err
	}
	if string(head[:4]) != "HFS0" {
		return nil, errors.New("the game card has no HFS0 where its header says")
	}
	count := int64(binary.LittleEndian.Uint32(head[4:8]))
	names := int64(binary.LittleEndian.Uint32(head[8:12]))
	if count > 1<<16 || names > 4<<20 {
		return nil, errors.New("the HFS0 of the game card is damaged")
	}
	table := make([]byte, count*0x40+names)
	if err := readAt(reader, table, offset+0x10); err != nil {
		return nil, err
	}
	data := 0x10 + int64(len(table))
	files := make([]hfs0File, 0, count)
	for i := int64(0); i < count; i++ {
		entry := table[i*0x40:]
		at := int64(binary.LittleEndian.Uint32(entry[16:20]))
		if at >= names {
			return nil, errors.New("the HFS0 of the game card is damaged")
		}
		name := table[count*0x40+at:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		files = append(files, hfs0File{name: string(name), offset: data + int64(binary.LittleEndian.Uint64(entry[0:8]))})
	}
	return files, nil
}

// readAt reads all of p at offset: a read that ends at the end of the file is whole too.
func readAt(reader io.ReaderAt, p []byte, offset int64) error {
	n, err := reader.ReadAt(p, offset)
	if n == len(p) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}

// defaultPackTemplate is the name of the pack of each game, as Switch Library Manager names a
// SuperNSP: Game [0100...][v65536] (1G+1U+2D)
const defaultPackTemplate = "{name} [{id}][v{version}] ({contents})"

// gameGroups puts the files of each game together, in the order the games come and with the
// game itself first; the files that cannot be read are given apart, with why.
func gameGroups(sources []string) ([]uint64, map[uint64][]string, map[string]error) {
	return groupGames(context.Background(), sources)
}

// groupGames is gameGroups that stops reading the files once ctx ends.
func groupGames(ctx context.Context, sources []string) ([]uint64, map[uint64][]string, map[string]error) {
	groups := map[uint64][]string{}
	order := []uint64{}
	first := map[string]int{}
	unread := map[string]error{}
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		titles, err := readTitles(source)
		if err != nil {
			unread[source] = err
			continue
		}
		main := describeTitles(titles)
		id := uint64(0)
		for _, title := range titles {
			if strings.EqualFold(title.TitleId, main.TitleID) {
				id, _ = switchfs.GameTitleID(title.TitleId, title.Type)
			}
		}
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], source)
		first[source] = map[string]int{"BASE": 0, "UPD": 1, "DLC": 2}[main.Type]
	}
	for _, files := range groups {
		sort.SliceStable(files, func(i, j int) bool { return first[files[i]] < first[files[j]] })
	}
	return order, groups, unread
}

// packedAlready is a pack of the game id found where its pack would go (in -o, else in next, see
// packFolder): a file not given whose name has the title ID of the game and that holds more than
// one title. "" when there is none.
func packedAlready(o options, id uint64, files []string, next string) string {
	folder, err := outputFolder(o)
	if err != nil {
		return ""
	}
	if folder == "" {
		folder = next
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	given := map[string]bool{}
	for _, file := range files {
		given[strings.ToLower(filepath.Clean(file))] = true
	}
	tag := fmt.Sprintf("[%016X]", id)
	for _, entry := range entries {
		name := entry.Name()
		extension := strings.ToLower(filepath.Ext(name))
		if entry.IsDir() || !strings.Contains(strings.ToUpper(name), tag) || (extension != ".nsp" && extension != ".nsz" && extension != ".xci" && extension != ".xcz") {
			continue
		}
		path := filepath.Join(folder, name)
		if given[strings.ToLower(filepath.Clean(path))] {
			continue
		}
		if titles, err := readTitles(path); err == nil && len(titles) > 1 {
			return path
		}
	}
	return ""
}

// packGames packs the files given by game: each game with its updates and DLC in a SuperNSP of
// its own (into its game card, when it is one), named by o.packName, next to the files of the
// game (next to its folder, when it came in a folder given: see packFolder) or in -o. A game with
// one file only is skipped: there is nothing to pack it with. It gives how many files failed.
func packGames(ctx context.Context, o options, out output) int {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	failed := len(missing)
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	ctx = withDestinations(ctx, sources)
	order, groups, unread := groupGames(ctx, sources)
	for _, source := range sources {
		if err, ok := unread[source]; ok {
			out.done(source, "", "", shown(err))
			failed++
		}
	}
	template := packTemplate(o)
	folders := givenFolders(o)
	for _, id := range order {
		waitWhilePaused(out)
		if ctx.Err() != nil {
			break
		}
		files := groups[id]
		next := packFolder(files, sources, folders)
		// a game left out in the window, and a game packed before, unless asked to write again
		if o.skipGames[fmt.Sprintf("%016X", id)] {
			for _, file := range files {
				out.done(file, "", "", skipped("left out of the games to combine"))
			}
			continue
		}
		if packed := packedAlready(o, id, files, next); packed != "" && !o.overwrite {
			for _, file := range files {
				out.done(file, "", "", skipped("already combined in "+filepath.Base(packed)+" (-w writes it again)"))
			}
			continue
		}
		if len(files) == 1 {
			out.done(files[0], "", "", skipped("it is the only file of its game here: there is nothing to combine it with"))
			continue
		}
		failed += combine(ctx, o, files, template, next, out)
	}
	return failed
}

// packOne packs every file given into one SuperNSP (into the game card, when one is given),
// named by o.packName, next to the first file of the first game (next to the folder of the files,
// when they came in a folder given: see packFolder) or in -o: the files of each game together,
// the game itself first. Nothing is written when a file given cannot be read:
// the pack would lack it. It gives how many files failed.
func packOne(ctx context.Context, o options, out output) int {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	ctx = withDestinations(ctx, sources)
	order, groups, unread := groupGames(ctx, sources)
	files := []string{}
	for _, id := range order {
		files = append(files, groups[id]...)
	}
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	for _, source := range sources {
		if err, ok := unread[source]; ok {
			out.done(source, "", "", shown(err))
		}
	}
	if ctx.Err() != nil {
		return len(missing) + len(unread)
	}
	if len(missing)+len(unread) > 0 {
		for _, file := range files {
			out.done(file, "", "", errors.New("not combined: a file given could not be read, and the pack would lack it"))
		}
		return len(missing) + len(sources)
	}
	if len(files) < 2 {
		for _, file := range files {
			out.done(file, "", "", errors.New("a pack needs at least two files"))
		}
		return len(files)
	}
	return combine(ctx, o, files, packTemplate(o), packFolder(files, files, givenFolders(o)), out)
}

// packTemplate is the name of the packs of --pack-games and --pack-one.
func packTemplate(o options) string {
	if o.packName == "" {
		return defaultPackTemplate
	}
	return o.packName
}

// combine writes the pack of files, in -o or else in next, and tells each of them: the first one
// with the pack, the others combined into it; the originals go (with --rm-source) once the pack
// holds everything they hold. It gives how many files failed.
func combine(ctx context.Context, o options, files []string, template string, next string, out output) int {
	name, note, check, err := packGame(ctx, o, files, template, next, out)
	err = shown(err)
	if isSkipped(err) {
		for _, file := range files {
			out.done(file, "", "", err)
		}
		return 0
	}
	kept := ""
	if err == nil {
		kept = incompleteNote(o, check, name, files)
	}
	out.done(files[0], name, joinNotes(keptNote(o, check, note), kept), err)
	for _, file := range files[1:] {
		if err != nil {
			out.done(file, "", "", err)
			continue
		}
		out.done(file, "", "combined into "+filepath.Base(name), nil)
	}
	if err != nil {
		return len(files)
	}
	if kept != "" {
		return 0
	}
	failed := 0
	for _, file := range files {
		if _, _, err := removeSource(o, file, check, name, ""); err != nil {
			out.done(file, "", "", shown(err))
			failed++
		}
	}
	return failed
}

// packGame writes the pack of the files of a game, named by the titles it holds and its size,
// in next (see packFolder) or in the output folder, and tells what it did. Its own pack, made
// before and given among its files, is told (skipped) unless -w writes it again; then the new
// pack takes its place only when it holds every title it keeps whole.
func packGame(ctx context.Context, o options, files []string, template string, next string, out output) (string, string, switchfs.FileCheck, error) {
	extension, err := packKind(files)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	if again := packedAmong(o, files, template, extension, next); again != "" && !o.overwrite {
		return "", "", switchfs.FileCheck{}, skipped("already combined in " + filepath.Base(again) + " (-w writes it again)")
	}
	// written under a hidden name of its own first, in the folder of the pack: its name tells
	// its size, known once it is written
	folder := next
	if o.output != "" {
		output, err := outputFolder(o)
		if err == nil {
			err = os.MkdirAll(output, 0o755)
		}
		if err != nil {
			return "", "", switchfs.FileCheck{}, err
		}
		folder = output
	}
	written := filepath.Join(folder, fmt.Sprintf(".nxpress-pack-%d%s", time.Now().UnixNano(), extension))
	defer os.Remove(written)
	note, check, err := writePack(ctx, files, written, files[0], out)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	info, err := os.Stat(written)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	name, err := packName(o, files, template, extension, info.Size(), next)
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	again := ""
	for _, file := range files {
		if sameFile(file, name) {
			again = file
		}
	}
	if again == "" {
		err = claim(ctx, files[0], name)
	} else if lacks, lackErr := packLacks(written, files); lackErr != nil || len(lacks) > 0 {
		// its own pack is written again only with all it holds
		err = lackErr
		if err == nil {
			err = fmt.Errorf("the new pack lacks %s: %s is kept as it is", strings.Join(lacks, ", "), filepath.Base(again))
		}
	}
	if err == nil {
		err = free(o, name)
	}
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", switchfs.FileCheck{}, err
	}
	return name, note, check, nil
}

// packName is the name of the pack of the files of a game: by the newest version of each
// title they hold and its size, in next (see packFolder) or in the output folder.
func packName(o options, files []string, template string, extension string, size int64, next string) (string, error) {
	titles := []*switchfs.ContentMetaAttributes{}
	newest := map[string]*switchfs.ContentMetaAttributes{}
	for _, file := range files {
		read, err := readTitles(file)
		if err != nil {
			return "", err
		}
		for _, title := range read {
			if kept, ok := newest[strings.ToLower(title.TitleId)]; !ok || title.Version > kept.Version {
				newest[strings.ToLower(title.TitleId)] = title
			}
		}
	}
	for _, title := range newest {
		titles = append(titles, title)
	}
	title := describeTitles(titles)
	title.Size = size
	return target(o, filepath.Join(next, strings.TrimSuffix(newName(files[0], title, template), filepath.Ext(files[0]))+extension))
}

// packedAmong is a file of a game that is its pack already, given with the others: one that
// holds more than one title, under the name the pack of them would get. "" when there is none.
func packedAmong(o options, files []string, template string, extension string, next string) string {
	for _, file := range files {
		titles, err := readTitles(file)
		info, statErr := os.Stat(file)
		if err != nil || statErr != nil || len(titles) < 2 {
			continue
		}
		// with what it holds, its own size is the size of the pack
		if name, err := packName(o, files, template, extension, info.Size(), next); err == nil && sameFile(name, file) {
			return file
		}
	}
	return ""
}

// givenFolders are the folders a work was given, as absolute paths: among the files and folders
// named on the command line or dropped or chosen in the window (o.origins), a folder of parts
// being a file.
func givenFolders(o options) []string {
	folders := []string{}
	for _, path := range append(append([]string{}, o.origins...), o.files...) {
		if _, parts := splitContainer(path); parts {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if absolute, err := filepath.Abs(path); err == nil {
				folders = append(folders, absolute)
			}
		}
	}
	return folders
}

// packFolder is where the pack of files goes when no output folder is given (#45). Files that
// came in a folder given are what that folder holds: their pack goes next to the folder that
// holds them all, in the one above it, when that folder holds no other file of the work; files
// from several folders given go next to those, in the folder that holds them. Else, as for files
// given one by one, it goes next to the first file. The packs made before among the files do not
// count for where the files lie, so a pack made again is found where it went the first time.
func packFolder(files []string, work []string, folders []string) string {
	next := filepath.Dir(files[0])
	if len(folders) == 0 {
		return next
	}
	own := []string{}
	for _, file := range files {
		if titles, err := readTitles(file); err != nil || len(titles) < 2 {
			own = append(own, file)
		}
	}
	if len(own) == 0 {
		own = files
	}
	given := func(path string) bool {
		for _, folder := range folders {
			if within(path, folder) {
				return true
			}
		}
		return false
	}
	holder := commonFolder(own)
	if holder == "" {
		return next
	}
	beside := filepath.Dir(holder)
	if !given(holder) {
		for _, file := range own {
			if absolute, err := filepath.Abs(file); err != nil || !given(absolute) {
				return next
			}
		}
		beside = holder
	} else if beside == holder {
		// the root of a drive has no folder above it
		return next
	}
	packed := map[string]bool{}
	for _, file := range files {
		if absolute, err := filepath.Abs(file); err == nil {
			packed[strings.ToLower(absolute)] = true
		}
	}
	for _, file := range work {
		if absolute, err := filepath.Abs(file); err == nil && !packed[strings.ToLower(absolute)] && within(absolute, holder) {
			return next
		}
	}
	return beside
}

// commonFolder is the folder that holds all the files given, as an absolute path; "" when they
// are on different drives.
func commonFolder(files []string) string {
	holder := ""
	for _, file := range files {
		absolute, err := filepath.Abs(file)
		if err != nil {
			return ""
		}
		if holder == "" {
			holder = filepath.Dir(absolute)
		}
		for !within(absolute, holder) {
			above := filepath.Dir(holder)
			if above == holder {
				return ""
			}
			holder = above
		}
	}
	return holder
}

// within tells whether path is folder or lies in it, at any depth; both are absolute.
func within(path string, folder string) bool {
	relative, err := filepath.Rel(folder, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// packKind is the extension of the pack of the files given: an NSP, an NSZ when one of them
// holds NCZ files, and with a game card among them an XCI (or an XCZ).
func packKind(sources []string) (string, error) {
	cards, _, err := splitCards(sources)
	if err != nil {
		return "", err
	}
	compressed := false
	for _, source := range sources {
		if isEmpty(source) {
			return "", fmt.Errorf("%s: %w", filepath.Base(source), errEmpty)
		}
		if facts, err := switchfs.DescribeFile(source); err != nil {
			return "", err
		} else if facts.Solid+facts.Blocks > 0 {
			compressed = true
		}
	}
	switch {
	case len(cards) == 1 && compressed:
		return ".xcz", nil
	case len(cards) == 1:
		return ".xci", nil
	case compressed:
		return ".nsz", nil
	}
	return ".nsp", nil
}

// splitCards puts the game card among the files apart: its updates and DLC go into it, and the
// pack is a game card.
func splitCards(sources []string) ([]string, []string, error) {
	cards := []string{}
	others := []string{}
	for _, source := range sources {
		if extension := strings.ToLower(filepath.Ext(source)); extension == ".xci" || extension == ".xcz" {
			cards = append(cards, source)
		} else {
			others = append(others, source)
		}
	}
	if len(cards) > 1 {
		return nil, nil, errors.New("only one game card can be packed: give its updates and DLC as NSP or NSZ")
	}
	return cards, others, nil
}

// writePack writes the pack of the files given to written, checked as it is written, and tells
// what it did. label names the work in its progress.
func writePack(ctx context.Context, sources []string, written string, label string, out output) (string, switchfs.FileCheck, error) {
	cards, others, err := splitCards(sources)
	if err != nil {
		return "", switchfs.FileCheck{}, err
	}
	// of a title given twice (an update in a SuperNSP, and a newer one), the newest is kept
	var (
		skipped  int64
		replaced []string
		check    switchfs.FileCheck
	)
	if len(cards) == 1 {
		replaced, check, err = switchfs.PackIntoCard(ctx, cards[0], others, written, out.progress(label, "Packing"))
	} else {
		skipped, replaced, check, err = switchfs.MergeNewestAndCheck(ctx, sources, written, true, out.progress(label, "Packing"))
	}
	if err != nil {
		return "", switchfs.FileCheck{}, err
	}
	note := fmt.Sprintf("%d files merged", len(sources))
	if skipped > 0 {
		note += fmt.Sprintf(", %s of delta fragments left out", size(skipped))
	}
	if len(replaced) > 0 {
		note += ", newer versions in place of " + strings.Join(replaced, ", ")
	}
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	return note, check, nil
}
