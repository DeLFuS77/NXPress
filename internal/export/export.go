// Package export writes a table to a file a spreadsheet opens (#89): CSV, or XLSX for Excel and
// the programs that read it, with no library of its own: one sheet, its header in bold and kept
// in sight, a filter on each column, and numbers as numbers.
package export

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DeLFuS77/NXPress/internal/durable"
)

// Table is what is written: a sheet with a name, its header, and its rows. A cell is a string,
// or a number: an int64 or a float64 (as JSON gives numbers) as it is, or Bytes, written with
// the thousands apart.
type Table struct {
	Sheet  string
	Header []string
	Rows   [][]any
}

// Bytes is a size, written with the thousands apart so it reads at a glance and still adds up.
type Bytes int64

// Write writes the table to path, as XLSX when its name ends so, else as CSV: under a hidden
// name first, then under its own, in place of the file there (the dialog asked).
func Write(path string, table Table) error {
	var data bytes.Buffer
	var err error
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		err = WriteXLSX(&data, table)
	} else {
		err = WriteCSV(&data, table)
	}
	if err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), fmt.Sprintf(".%s.%d.nxpress", filepath.Base(path), time.Now().UnixNano()))
	if err := os.WriteFile(temporary, data.Bytes(), 0o644); err != nil {
		return err
	}
	if err := durable.CommitFile(temporary, path); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}

// text is a cell as text, for CSV.
func text(cell any) string {
	switch value := cell.(type) {
	case nil:
		return ""
	case string:
		return value
	case int64:
		return strconv.FormatInt(value, 10)
	case int:
		return strconv.Itoa(value)
	case Bytes:
		return strconv.FormatInt(int64(value), 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return fmt.Sprint(cell)
}

// WriteCSV writes the table as CSV in UTF-8, with the mark that tells Excel so (else it reads
// names in Japanese or with accents wrong), lines ending as on Windows. A text a spreadsheet
// would take for a formula starts with ' so it stays text: a file named "=…" or "+…".
func WriteCSV(w io.Writer, table Table) error {
	if _, err := w.Write([]byte("\uFEFF")); err != nil {
		return err
	}
	out := csv.NewWriter(w)
	out.UseCRLF = true
	for _, row := range append([][]any{toCells(table.Header)}, table.Rows...) {
		record := make([]string, len(row))
		for i, cell := range row {
			record[i] = text(cell)
			if _, isText := cell.(string); isText && record[i] != "" && strings.ContainsRune("=+-@\t\r", rune(record[i][0])) {
				record[i] = "'" + record[i]
			}
		}
		if err := out.Write(record); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}

func toCells(texts []string) []any {
	cells := make([]any, len(texts))
	for i, text := range texts {
		cells[i] = text
	}
	return cells
}

// column is the name of a column: A to Z, then AA, AB…
func column(index int) string {
	name := ""
	for index++; index > 0; index = (index - 1) / 26 {
		name = string(rune('A'+(index-1)%26)) + name
	}
	return name
}

// xmlText is a text as XML holds it: escaped, and without the characters XML cannot hold.
func xmlText(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch {
		case r == utf8.RuneError || r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xFFFE || r == 0xFFFF:
			continue
		case r == '&':
			out.WriteString("&amp;")
		case r == '<':
			out.WriteString("&lt;")
		case r == '>':
			out.WriteString("&gt;")
		case r == '"':
			out.WriteString("&quot;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// sheetName is a name Excel takes for a sheet: 31 characters at most, none of []:*?/\.
func sheetName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if runes := []rune(name); len(runes) > 31 {
		name = string(runes[:31])
	}
	if name == "" {
		return "Sheet1"
	}
	return name
}

const (
	mainNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	relNamespace  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	xmlHead       = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
)

// the styles: 0 plain, 1 bold for the header, 2 a number with the thousands apart
const styles = xmlHead + `<styleSheet xmlns="` + mainNamespace + `">` +
	`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
	`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
	`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="3"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
	`<xf numFmtId="3" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs>` +
	`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`

// WriteXLSX writes the table as an Excel workbook of one sheet.
func WriteXLSX(w io.Writer, table Table) error {
	name := sheetName(table.Sheet)
	columns := len(table.Header)
	for _, row := range table.Rows {
		columns = max(columns, len(row))
	}
	last := column(max(columns, 1)-1) + strconv.Itoa(len(table.Rows)+1)
	parts := [][2]string{
		{"[Content_Types].xml", xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relNamespace + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", xmlHead + `<workbook xmlns="` + mainNamespace + `" xmlns:r="` + relNamespace + `">` +
			`<sheets><sheet name="` + xmlText(name) + `" sheetId="1" r:id="rId1"/></sheets>` +
			`<definedNames><definedName name="_xlnm._FilterDatabase" localSheetId="0" hidden="1">'` + xmlText(strings.ReplaceAll(name, "'", "''")) + `'!$A$1:$` + column(max(columns, 1)-1) + `$` + strconv.Itoa(len(table.Rows)+1) + `</definedName></definedNames></workbook>`},
		{"xl/_rels/workbook.xml.rels", xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relNamespace + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + relNamespace + `/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", styles},
		{"xl/worksheets/sheet1.xml", sheet(table, columns, last)},
	}
	archive := zip.NewWriter(w)
	for _, part := range parts {
		file, err := archive.CreateHeader(&zip.FileHeader{Name: part[0], Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(file, part[1]); err != nil {
			return err
		}
	}
	return archive.Close()
}

// sheet is the worksheet: the header kept in sight, each column as wide as its longest text
// (within reason), and a filter on the header.
func sheet(table Table, columns int, last string) string {
	widths := make([]int, columns)
	all := append([][]any{toCells(table.Header)}, table.Rows...)
	for _, row := range all {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(text(cell))+2)
		}
	}
	var out strings.Builder
	out.WriteString(xmlHead + `<worksheet xmlns="` + mainNamespace + `" xmlns:r="` + relNamespace + `">`)
	out.WriteString(`<dimension ref="A1:` + last + `"/>`)
	out.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	if columns > 0 {
		out.WriteString(`<cols>`)
		for i, width := range widths {
			fmt.Fprintf(&out, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, min(max(width, 8), 60))
		}
		out.WriteString(`</cols>`)
	}
	out.WriteString(`<sheetData>`)
	for r, row := range all {
		fmt.Fprintf(&out, `<row r="%d">`, r+1)
		for c, cell := range row {
			ref := column(c) + strconv.Itoa(r+1)
			style := ""
			if r == 0 {
				style = ` s="1"`
			}
			switch value := cell.(type) {
			case nil:
				continue
			case int64, int, float64:
				fmt.Fprintf(&out, `<c r="%s"%s><v>%s</v></c>`, ref, style, text(value))
			case Bytes:
				fmt.Fprintf(&out, `<c r="%s" s="2"><v>%d</v></c>`, ref, int64(value))
			default:
				content := xmlText(text(value))
				if content == "" {
					continue
				}
				space := ""
				if strings.TrimSpace(content) != content {
					space = ` xml:space="preserve"`
				}
				fmt.Fprintf(&out, `<c r="%s" t="inlineStr"%s><is><t%s>%s</t></is></c>`, ref, style, space, content)
			}
		}
		out.WriteString(`</row>`)
	}
	out.WriteString(`</sheetData>`)
	if columns > 0 {
		out.WriteString(`<autoFilter ref="A1:` + last + `"/>`)
	}
	out.WriteString(`</worksheet>`)
	return out.String()
}
