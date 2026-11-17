package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrouped(t *testing.T) {
	for number, want := range map[int64]string{0: "0", 12: "12", 999: "999", 1000: "1,000", 15111444614: "15,111,444,614", -1234567: "-1,234,567"} {
		if got := grouped(number); got != want {
			t.Errorf("grouped(%d) = %q, want %q", number, got, want)
		}
	}
	if got := bytesText(1536); got != "1.5 KB (1,536 bytes)" {
		t.Errorf("bytesText = %q", got)
	}
}

// a file that cannot be read keeps its place in the report, with the reason, and does not stop the rest
func TestReportFilesKeepsUnreadable(t *testing.T) {
	folder := t.TempDir()
	missing := filepath.Join(folder, "gone.nsp")
	empty := filepath.Join(folder, "empty.nsp")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	text := ReportFiles([]string{missing, empty}, nil)
	for _, want := range []string{"NXPress Details", "[1/2]", "[2/2]", "gone.nsp", "empty.nsp", "Problem:"} {
		if !strings.Contains(text, want) {
			t.Errorf("the report lacks %q:\n%s", want, text)
		}
	}
}

// the report tells UPDATE, not UPD, and does not tell the folder of a file
func TestReportWords(t *testing.T) {
	for kind, want := range map[string]string{"BASE": "BASE", "UPD": "UPDATE", "DLC": "DLC"} {
		if got := kindWord(kind); got != want {
			t.Errorf("kindWord(%q) = %q, want %q", kind, got, want)
		}
	}
	text := ReportFiles([]string{filepath.Join(t.TempDir(), "gone.nsp")}, nil)
	if strings.Contains(text, "Folder:") {
		t.Errorf("the report tells the folder of a file:\n%s", text)
	}
}
