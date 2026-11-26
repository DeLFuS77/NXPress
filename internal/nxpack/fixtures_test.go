package nxpack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// TestRepackFixtures packs again every version of the .nx files of the conformance tests of the
// format (NX_FIXTURES names their folder) and checks that the identity of each, which hashes its
// kind, version and whole tree, is the one the reference packer gave it.
func TestRepackFixtures(t *testing.T) {
	dir := os.Getenv("NX_FIXTURES")
	if dir == "" {
		t.Skip("NX_FIXTURES is not set")
	}
	names, _ := filepath.Glob(filepath.Join(dir, "*.nx"))
	checked := 0
	for _, path := range names {
		data, err := switchfs.OpenNxData(path)
		if err != nil {
			continue
		}
		if data.Verify(t.Context(), nil) != nil {
			data.Close()
			continue
		}
		for a, app := range data.Applications {
			for s, snapshot := range app.Snapshots {
				if snapshot.ContentId != [16]byte{} && snapshot.AddonTitleId == 0 {
					continue // a native content ID: the packer writes only the ID of a DLC there
				}
				folder := t.TempDir()
				v := Snapshot{Kind: byte(snapshot.Type), Version: snapshot.Version, Program: snapshot.ProgramId, Addon: snapshot.AddonTitleId}
				err := data.Walk(a, s, func(p string) error {
					if p != "" {
						v.Dirs = append(v.Dirs, p)
					}
					return nil
				}, func(p string, index int) error {
					source := filepath.Join(folder, filepath.FromSlash(p))
					os.MkdirAll(filepath.Dir(source), 0o755)
					var buffer bytes.Buffer
					if err := data.WriteFile(t.Context(), index, &buffer, nil); err != nil {
						return err
					}
					v.Files = append(v.Files, File{Path: p, Source: source, Size: int64(buffer.Len())})
					return os.WriteFile(source, buffer.Bytes(), 0o644)
				})
				if err != nil {
					t.Fatal(err)
				}
				if snapshot.Type == switchfs.NxSnapshotDlc && snapshot.AddonTitleId == 0 {
					continue // no title ID to write it with
				}
				out := filepath.Join(t.TempDir(), "again.nx")
				if _, err := Write(t.Context(), out, []Game{{ID: app.Id, Platform: byte(app.Platform), Versions: []Snapshot{v}}}, Options{}, nil); err != nil {
					t.Errorf("%s version %d: %v", filepath.Base(path), s, err)
					continue
				}
				referenceCheck(t, out)
				again, err := switchfs.ReadNx(out)
				if err != nil {
					t.Fatalf("%s version %d: %v", filepath.Base(path), s, err)
				}
				if got := again.Applications[0].Snapshots[0].Identity; got != snapshot.Identity {
					t.Errorf("%s version %d: identity %x, want %x", filepath.Base(path), s, got, snapshot.Identity)
				}
				checked++
			}
		}
		data.Close()
	}
	if checked == 0 {
		t.Fatal("no version checked")
	}
	t.Logf("%d versions packed again with the same identity", checked)
}
