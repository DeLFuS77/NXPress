package jobs

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// --convert --to-xci turns NSP files into XCI files that hold the same titles, and takes no
// game card; --to-xci alone is told to have no effect (#127).
func TestConvertToCard(t *testing.T) {
	testkit.Isolate(t)
	o, err := ParseOptions([]string{"--convert", "--to-xci", "x.nsp"})
	if err != nil || o.Action() != "tocard" {
		t.Fatalf("options: %v %v", o.Action(), err)
	}
	alone, err := ParseOptions([]string{"--to-xci", "-C", "x.nsp"})
	if err == nil {
		err = alone.Check()
	}
	if err != nil || len(alone.Notes) == 0 {
		t.Fatalf("--to-xci alone: %v %v", alone.Notes, err)
	}
	folder := t.TempDir()
	base, update, _ := testkit.Game(t, folder)
	packed := filepath.Join(folder, "super.nsp")
	if _, _, err := switchfs.MergeAndCheck(context.Background(), []string{base, update}, packed, true, nil); err != nil {
		t.Fatal(err)
	}
	out := &testkit.Results{}
	if failed := RunJobs(context.Background(), Options{Convert: true, ToCard: true, Multi: 1, Files: []string{packed}}, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	card := filepath.Join(folder, "super.xci")
	want, err := ReadTitles(packed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadTitles(card)
	versions := func(titles []*switchfs.ContentMetaAttributes) map[string]int {
		m := map[string]int{}
		for _, title := range titles {
			m[title.TitleId] = title.Version
		}
		return m
	}
	if err != nil || len(want) != 2 || !reflect.DeepEqual(versions(got), versions(want)) {
		t.Fatalf("the card holds %v, %v; the NSP %v", versions(got), err, versions(want))
	}
	// a game card given to it is not taken
	again := &testkit.Results{}
	RunJobs(context.Background(), Options{Convert: true, ToCard: true, Multi: 1, Files: []string{folder}}, again)
	for path := range again.Results {
		if filepath.Ext(path) == ".xci" {
			t.Fatalf("a game card was converted: %s", path)
		}
	}
}
