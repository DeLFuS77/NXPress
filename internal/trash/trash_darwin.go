package trash

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// The Trash of macOS: ~/.Trash for the volume of the home folder, and .Trashes/<uid> at the top of
// any other volume, as the Finder keeps them. An item of a name the Trash holds already gets a
// number, as the Finder gives it. A network volume has none: the Finder deletes from it for good.

func move(path string) error {
	dir, err := find(path)
	if err != nil {
		return err
	}
	return put(dir, path)
}

func check(path string) error {
	_, err := find(path)
	return err
}

// find is the Trash of the volume of path.
func find(path string) (string, error) {
	folder, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	var volume unix.Statfs_t
	if err := unix.Statfs(folder, &volume); err != nil {
		return "", err
	}
	mount := unix.ByteSliceToString(volume.Mntonname[:])
	// a network volume, or one that cannot be written, has no Trash
	if volume.Flags&unix.MNT_LOCAL == 0 || volume.Flags&unix.MNT_RDONLY != 0 {
		return "", &Unavailable{Reason: NoBin, Drive: mount}
	}
	if home, err := os.UserHomeDir(); err == nil {
		var ofHome unix.Statfs_t
		if unix.Statfs(home, &ofHome) == nil && unix.ByteSliceToString(ofHome.Mntonname[:]) == mount {
			dir := filepath.Join(home, ".Trash")
			if ready(dir) {
				return dir, nil
			}
			return "", &Unavailable{Reason: NoBin, Drive: mount}
		}
	}
	// .Trashes at the top of the volume, which anyone may go through and nobody may list (as the
	// Finder makes it), and the folder of the user in it
	trashes := filepath.Join(mount, ".Trashes")
	if err := os.Mkdir(trashes, 0o1333); err == nil {
		os.Chmod(trashes, 0o1333|os.ModeSticky)
	}
	if info, err := os.Lstat(trashes); err != nil || !info.IsDir() {
		return "", &Unavailable{Reason: NoBin, Drive: mount}
	}
	dir := filepath.Join(trashes, strconv.Itoa(os.Getuid()))
	if !ready(dir) {
		return "", &Unavailable{Reason: NoBin, Drive: mount}
	}
	return dir, nil
}

// ready makes a Trash folder when it is not there, for the user only, and tells whether it is a
// folder that can be written.
func ready(dir string) bool {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return false
	}
	info, err := os.Lstat(dir)
	return err == nil && info.IsDir() && unix.Access(dir, unix.W_OK) == nil
}

// put moves path into the Trash, under a name no other item there has.
func put(dir string, path string) error {
	for n := 1; n < 10000; n++ {
		target := filepath.Join(dir, numbered(filepath.Base(path), n))
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		// the name is taken without replacing what took it meanwhile
		err := unix.RenamexNp(path, target, unix.RENAME_EXCL)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if errors.Is(err, syscall.EXDEV) {
			return &Unavailable{Reason: NoBin, Drive: dir}
		}
		return err
	}
	return errors.New("the Trash has too many items of that name")
}

// numbered is the name of the nth item of a name in the Trash: name itself, then "name 2.ext",
// "name 3.ext"...
func numbered(name string, n int) string {
	if n == 1 {
		return name
	}
	extension := filepath.Ext(name)
	if extension == name {
		extension = ""
	}
	return strings.TrimSuffix(name, extension) + " " + strconv.Itoa(n) + extension
}
