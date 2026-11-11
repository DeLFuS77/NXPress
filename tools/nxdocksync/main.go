// Command nxdocksync copies the code of NXDock that "Optimize for install" runs (internal/optimize/native)
// from a commit of the NXDock repository, and writes the list of what it copied with the SHA-256 of
// each file (NXDOCK.sum), which a test checks: the copies are never edited here, a change goes to
// NXDock first and comes back with this command.
//
//	go run ./tools/nxdocksync -from ../NXDock [-rev main]
//
// Only what is committed is copied (git show), so the list names a commit that has those files.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Files are the files copied: where they are in NXDock, and the name they take in native.
var Files = [][2]string{
	{"source/patch_view.hpp", "patch_view.hpp"},
	{"source/patch_view.cpp", "patch_view.cpp"},
	{"source/romfs_lz4.hpp", "romfs_lz4.hpp"},
	{"source/romfs_lz4.cpp", "romfs_lz4.cpp"},
	{"source/merge_pack.hpp", "merge_pack.hpp"},
	{"source/merge_pack.cpp", "merge_pack.cpp"},
	{"source/merge_nca.hpp", "merge_nca.hpp"},
	{"source/merge_nca.cpp", "merge_nca.cpp"},
	{"source/merge_game.hpp", "merge_game.hpp"},
	{"source/merge_game.cpp", "merge_game.cpp"},
	{"source/merge_meta.hpp", "merge_meta.hpp"},
	{"source/merge_meta.cpp", "merge_meta.cpp"},
	{"source/lz4nintendo.c", "lz4nintendo.c"},
	{"include/lz4nintendo.h", "lz4nintendo.h"},
}

func main() {
	from := flag.String("from", "../NXDock", "the NXDock repository")
	rev := flag.String("rev", "HEAD", "the commit to copy from")
	to := flag.String("to", filepath.Join("internal", "optimize", "native"), "where the copies go")
	flag.Parse()
	if err := run(*from, *rev, *to); err != nil {
		fmt.Fprintln(os.Stderr, "nxdocksync:", err)
		os.Exit(1)
	}
}

func git(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func run(from string, rev string, to string) error {
	commit, err := git(from, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return err
	}
	sum := strings.Builder{}
	fmt.Fprintf(&sum, "# The code of NXDock in this folder, copied by tools/nxdocksync: never edit it here.\ncommit %s\n", strings.TrimSpace(string(commit)))
	for _, f := range Files {
		data, err := git(from, "show", strings.TrimSpace(string(commit))+":"+f[0])
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(to, f[1]), data, 0o644); err != nil {
			return err
		}
		h := sha256.Sum256(data)
		fmt.Fprintf(&sum, "%s  %s  %s\n", hex.EncodeToString(h[:]), f[1], f[0])
	}
	return os.WriteFile(filepath.Join(to, "NXDOCK.sum"), []byte(sum.String()), 0o644)
}
