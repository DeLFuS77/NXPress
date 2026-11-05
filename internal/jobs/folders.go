package jobs

import (
	"os"
	"path/filepath"
	"sort"
)

// unreadFolder is a folder that could not be read, and why.
type unreadFolder struct {
	Path string
	Err  error
}

// FolderFiles are the files in a folder and its folders of parts, in order of name, of every
// kind (the caller takes the ones it works on); with recursive, the ones in its folders too,
// and in theirs. unread are the folders that could not be read, and nested tells whether
// folders were left out, without recursive.
func FolderFiles(folder string, recursive bool) (found []string, unread []unreadFolder, nested bool) {
	filepath.WalkDir(folder, func(walked string, entry os.DirEntry, err error) error {
		if err != nil {
			// told, not passed over: a folder without permission would leave its games out
			// without a word
			unread = append(unread, unreadFolder{Path: walked, Err: err})
			return nil
		}
		if walked == folder {
			return nil
		}
		if !entry.IsDir() {
			found = append(found, walked)
			return nil
		}
		// a folder of parts is one file, and a dump folder is one game: neither is looked into
		if _, ok := SplitContainer(walked); ok || IsDumpFolder(walked) {
			found = append(found, walked)
			return filepath.SkipDir
		}
		if !recursive {
			nested = true
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(found)
	return found, unread, nested
}
