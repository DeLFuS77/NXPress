// Package worklog keeps the log of the works done in the window (#69): every work with its
// files, in a list the window reads back and in a text file any editor opens, both in the
// folder of the settings of NXPress. Past the most works kept, the oldest are left out.
package worklog

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/durable"
)

// the names of the two files in the folder of the settings
const (
	ListName = "log.json"
	TextName = "log.txt"
)

// Row is a file of a work, as the window shows it.
type Row struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// a file a check found damaged, not one it could not check
	Damaged bool   `json:"damaged,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Work is a work of the window: when it started, what it did and what came of each file. Text
// is how it reads in the text file, written by the window in its language.
type Work struct {
	When      int64  `json:"when"`
	Mode      string `json:"mode"`
	Cancelled bool   `json:"cancelled,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Seconds   int    `json:"seconds"`
	Saved     int64  `json:"saved,omitempty"`
	Rows      []Row  `json:"rows"`
	Text      string `json:"text"`
}

// Read gives the works kept, the oldest first; none when there is no log, or it cannot be
// read: a log spoilt by hand must not stop the window.
func Read(folder string) []Work {
	works := []Work{}
	data, err := os.ReadFile(filepath.Join(folder, ListName))
	if err != nil || json.Unmarshal(data, &works) != nil {
		return []Work{}
	}
	return works
}

// Add puts a work at the end of the log, and leaves out the oldest past keep (no limit when
// keep is 0 or less). It gives the works kept.
func Add(folder string, work Work, keep int) ([]Work, error) {
	works := Trim(append(Read(folder), work), keep)
	return works, Write(folder, works)
}

// Trim leaves out the oldest works past keep; keep 0 or less keeps them all.
func Trim(works []Work, keep int) []Work {
	if keep > 0 && len(works) > keep {
		return append([]Work{}, works[len(works)-keep:]...)
	}
	return works
}

// Write puts the works in the two files, in place of what they held: a power cut leaves the
// files as they were or as they are now, never half written. No works removes both.
func Write(folder string, works []Work) error {
	if len(works) == 0 {
		return Clear(folder)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	list, err := json.Marshal(works)
	if err != nil {
		return err
	}
	var text bytes.Buffer
	for i, work := range works {
		if i > 0 {
			text.WriteString("\n")
		}
		text.WriteString(strings.TrimRight(work.Text, "\r\n"))
		text.WriteString("\n")
	}
	if err := writeFile(folder, TextName, text.Bytes()); err != nil {
		return err
	}
	return writeFile(folder, ListName, list)
}

// Clear removes the log: both files.
func Clear(folder string) error {
	var failed error
	for _, name := range []string{ListName, TextName} {
		if err := os.Remove(filepath.Join(folder, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			failed = err
		}
	}
	return failed
}

// writeFile writes a file of the log under a hidden name, then gives it its name.
func writeFile(folder string, name string, data []byte) error {
	temporary := filepath.Join(folder, "."+name+".nxpress")
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := durable.CommitFile(temporary, filepath.Join(folder, name)); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}
