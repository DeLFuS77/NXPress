package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The trash of the freedesktop.org Trash specification, which the desktops of Linux share: the
// home trash ($XDG_DATA_HOME/Trash) for the drive of the home folder, and $topdir/.Trash/$uid or
// $topdir/.Trash-$uid for the drive mounted on $topdir. Each item goes into files/, and a
// .trashinfo of the same name into info/ tells where it was and when it went, so the file managers
// put it back. A network folder has none.

func move(path string) error {
	where, err := find(path)
	if err != nil {
		return err
	}
	return where.put(path, time.Now())
}

func check(path string) error {
	_, err := find(path)
	return err
}

// place is a trash directory: files/ and info/ in dir, and the folder the paths in its
// .trashinfo files are written from ("" for the home trash, whose paths are absolute).
type place struct {
	dir    string
	topdir string
}

// find is the trash directory of path: the home trash on the drive of the home folder, else the
// one of the drive it is on.
func find(path string) (place, error) {
	// a link is moved itself: the folder it is in tells its drive
	folder, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return place{}, err
	}
	path = filepath.Join(folder, filepath.Base(path))
	topdir, err := mountPoint(path)
	if err != nil {
		return place{}, err
	}
	if network(path) {
		return place{}, &Unavailable{Reason: NoBin, Drive: topdir}
	}
	if home := homeTrash(); home != "" && sameDevice(path, home) {
		if ready(home) {
			return place{dir: home}, nil
		}
		return place{}, &Unavailable{Reason: NoBin, Drive: topdir}
	}
	if dir := topdirTrash(topdir, os.Getuid()); dir != "" {
		return place{dir: dir, topdir: topdir}, nil
	}
	return place{}, &Unavailable{Reason: NoBin, Drive: topdir}
}

// homeTrash is $XDG_DATA_HOME/Trash, or ~/.local/share/Trash.
func homeTrash() string {
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return filepath.Join(data, "Trash")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, ".local", "share", "Trash")
}

// sameDevice tells whether path is on the drive of dir, which need not be there yet: the drive
// of the nearest folder above it that is.
func sameDevice(path string, dir string) bool {
	var item, there unix.Stat_t
	if unix.Lstat(path, &item) != nil {
		return false
	}
	for {
		if err := unix.Stat(dir, &there); err == nil {
			return uint64(there.Dev) == uint64(item.Dev)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// topdirTrash is the trash of the drive mounted on topdir, for the user: $topdir/.Trash/$uid when
// the administrator made $topdir/.Trash (a folder, not a link, with the sticky bit), else
// $topdir/.Trash-$uid, made when it is not there; "" when neither can be used.
func topdirTrash(topdir string, uid int) string {
	shared := filepath.Join(topdir, ".Trash")
	if info, err := os.Lstat(shared); err == nil && info.IsDir() && info.Mode()&os.ModeSticky != 0 {
		if dir := filepath.Join(shared, strconv.Itoa(uid)); ready(dir) {
			return dir
		}
	}
	if dir := filepath.Join(topdir, ".Trash-"+strconv.Itoa(uid)); ready(dir) {
		return dir
	}
	return ""
}

// ready makes a trash directory and its files/ and info/ when they are not there, for the user
// only, and tells whether it is a folder of its own that can be written: not a link, and owned
// by the user.
func ready(dir string) bool {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	var stat unix.Stat_t
	if unix.Lstat(dir, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || int(stat.Uid) != os.Getuid() {
		return false
	}
	for _, inside := range []string{"files", "info"} {
		if err := os.Mkdir(filepath.Join(dir, inside), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return false
		}
		if info, err := os.Lstat(filepath.Join(dir, inside)); err != nil || !info.IsDir() {
			return false
		}
	}
	return unix.Access(filepath.Join(dir, "files"), unix.W_OK) == nil && unix.Access(filepath.Join(dir, "info"), unix.W_OK) == nil
}

// put moves path into the trash: its .trashinfo is made first, under a name no other item has,
// then the item takes that name in files/; the .trashinfo goes when it cannot.
func (p place) put(path string, when time.Time) error {
	original := path
	if p.topdir != "" {
		relative, err := filepath.Rel(p.topdir, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
			return &Unavailable{Reason: NoBin, Drive: p.topdir}
		}
		original = relative
	}
	content := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", escape(original), when.Format("2006-01-02T15:04:05"))
	for n := 1; n < 10000; n++ {
		name := numbered(filepath.Base(path), n)
		info := filepath.Join(p.dir, "info", name+".trashinfo")
		target := filepath.Join(p.dir, "files", name)
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		file, err := os.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = file.WriteString(content)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(path, target)
		}
		if err != nil {
			os.Remove(info)
			if errors.Is(err, syscall.EXDEV) {
				return &Unavailable{Reason: NoBin, Drive: p.topdir}
			}
			return err
		}
		return nil
	}
	return errors.New("the trash has too many items of that name")
}

// numbered is the name of the nth item of a name in the trash: name itself, then
// "name.2.ext", "name.3.ext"...
func numbered(name string, n int) string {
	if n == 1 {
		return name
	}
	extension := filepath.Ext(name)
	if extension == name {
		extension = ""
	}
	return strings.TrimSuffix(name, extension) + "." + strconv.Itoa(n) + extension
}

// escape writes a path as the specification asks, as a URL: every byte but the letters, the
// digits, "-._~" and "/" as %XX.
func escape(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', strings.IndexByte("-._~/", c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// mountPoint is the folder the drive of path is mounted on: the highest folder above it on the
// same drive.
func mountPoint(path string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	for {
		parent := filepath.Dir(dir)
		var above unix.Stat_t
		if parent == dir || unix.Stat(parent, &above) != nil || uint64(above.Dev) != uint64(stat.Dev) {
			return dir, nil
		}
		dir = parent
	}
}

// the file systems of network folders, by their magic number
var networkTypes = map[uint32]bool{
	0x6969:     true, // NFS
	0x517B:     true, // SMB
	0xFF534D42: true, // CIFS
	0xFE534D42: true, // SMB2
	0x5346414F: true, // AFS
	0x73757245: true, // Coda
	0x564C:     true, // NCP
	0x00C36400: true, // Ceph
}

// network tells whether path is on a network folder, which has no trash.
func network(path string) bool {
	var stat unix.Statfs_t
	if unix.Statfs(filepath.Dir(path), &stat) != nil {
		return false
	}
	return networkTypes[uint32(stat.Type)]
}
