package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sample = Table{
	Sheet:  "Details: [all]",
	Header: []string{"File", "Size", "Version", "Name"},
	Rows: [][]any{
		{"Game [0100000000010000][v0].nsp", Bytes(6210000000), float64(0), "Sky Garden"},
		{"=cmd.nsz", Bytes(1040), int64(393216), "Name, with a comma \"and quotes\""},
		{"日本語.xci", nil, int64(65536), " spaced \x01bell "},
	},
}

// CSV: in UTF-8 with its mark, lines ending as on Windows, numbers as numbers, quotes where a
// cell needs them, and a text a spreadsheet would take for a formula kept as text.
func TestCSV(t *testing.T) {
	var out bytes.Buffer
	if err := WriteCSV(&out, sample); err != nil {
		t.Fatal(err)
	}
	want := "\uFEFFFile,Size,Version,Name\r\n" +
		"Game [0100000000010000][v0].nsp,6210000000,0,Sky Garden\r\n" +
		"'=cmd.nsz,1040,393216,\"Name, with a comma \"\"and quotes\"\"\"\r\n" +
		"日本語.xci,,65536,\" spaced \x01bell \"\r\n"
	if out.String() != want {
		t.Errorf("got\n%q\nwant\n%q", out.String(), want)
	}
}

func TestColumn(t *testing.T) {
	for index, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := column(index); got != want {
			t.Errorf("%d: %s", index, got)
		}
	}
}

// XLSX: every part is well-formed XML; the sheet has its name made fit for Excel, the header in
// bold, numbers as numbers (sizes with the thousands apart), text as text with the characters
// XML cannot hold left out, the header kept in sight and a filter on it.
func TestXLSX(t *testing.T) {
	var out bytes.Buffer
	if err := WriteXLSX(&out, sample); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(reader)
		reader.Close()
		parts[file.Name] = string(data)
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			if _, err := decoder.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s is not well-formed: %v", file.Name, err)
			}
		}
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		if _, ok := parts[name]; !ok {
			t.Errorf("no %s", name)
		}
	}
	if !strings.Contains(parts["xl/workbook.xml"], `<sheet name="Details   all" sheetId="1"`) {
		t.Errorf("sheet name: %s", parts["xl/workbook.xml"])
	}
	sheet := parts["xl/worksheets/sheet1.xml"]
	for _, want := range []string{
		`<c r="A1" t="inlineStr" s="1"><is><t>File</t></is></c>`,
		`<c r="B2" s="2"><v>6210000000</v></c>`,
		`<c r="C3"><v>393216</v></c>`,
		`<c r="A3" t="inlineStr"><is><t>=cmd.nsz</t></is></c>`,
		`<t>Name, with a comma &quot;and quotes&quot;</t>`,
		`<c r="D4" t="inlineStr"><is><t xml:space="preserve"> spaced bell </t></is></c>`,
		`<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>`,
		`<autoFilter ref="A1:D4"/>`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("the sheet lacks %s", want)
		}
	}
	if strings.Contains(sheet, `r="B4"`) {
		t.Error("an empty cell written")
	}
}

// Write chooses the format by the name, replaces the file there, and leaves no file of its own.
func TestWrite(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"list.xlsx", "list.csv"} {
		path := filepath.Join(folder, name)
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Write(path, sample); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if xlsx := strings.HasSuffix(name, ".xlsx"); xlsx != bytes.HasPrefix(data, []byte("PK")) {
			t.Errorf("%s: written as the other format", name)
		}
	}
	entries, _ := os.ReadDir(folder)
	if len(entries) != 2 {
		t.Errorf("files left: %v", entries)
	}
}
