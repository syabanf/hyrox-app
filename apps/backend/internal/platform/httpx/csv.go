package httpx

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CSV in and out.
//
// Spreadsheets are how a shop's data actually arrives: a supplier sends a
// price list, somebody has three hundred products in a file, a manager wants
// last month's sales in something they can pivot. Reading and writing it
// belongs here rather than in each module, because the fiddly parts — a
// header row that says what the columns are, a number written "1.500.000",
// an import that must say what it would do before it does it — are the same
// everywhere.

// WriteCSV streams rows as a downloadable file.
func WriteCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-%s.csv"`, filename, time.Now().Format("20060102")))
	w.WriteHeader(http.StatusOK)

	writer := csv.NewWriter(w)
	// The BOM is what makes Excel open a UTF-8 file without mangling every
	// accented name in it. Without it, "Café" arrives as "CafÃ©".
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = writer.Write(header)
	for _, row := range rows {
		_ = writer.Write(row)
	}
	writer.Flush()
}

// CSVRow is one line of an upload, addressed by column name.
//
// By name rather than by position, because a spreadsheet somebody edited has
// its columns in whatever order they left them, and an importer that depends
// on order is an importer that silently puts prices in the name field.
type CSVRow struct {
	Line   int
	values map[string]string
}

// Get reads a column, empty when it is not there.
func (r CSVRow) Get(column string) string {
	return strings.TrimSpace(r.values[strings.ToLower(strings.TrimSpace(column))])
}

// Has reports whether a column was present at all, which is different from it
// being empty: a missing column is a file problem and an empty cell is a data
// one.
func (r CSVRow) Has(column string) bool {
	_, ok := r.values[strings.ToLower(strings.TrimSpace(column))]
	return ok
}

// Number reads a figure written the way people write them.
//
// "1.500.000", "1,500,000" and "1500000" are all one and a half million: an
// Indonesian spreadsheet uses dots for thousands and a comma for the decimal,
// an English one does the reverse, and refusing either is refusing most real
// files. The rule used is the last separator: whichever of . or , appears
// last with two or fewer digits after it is the decimal point.
func (r CSVRow) Number(column string) (float64, error) {
	raw := r.Get(column)
	if raw == "" {
		return 0, nil
	}

	cleaned := strings.Map(func(c rune) rune {
		if c == ' ' || c == '\u00a0' {
			return -1
		}
		return c
	}, raw)

	lastDot := strings.LastIndex(cleaned, ".")
	lastComma := strings.LastIndex(cleaned, ",")
	decimal := -1
	if lastDot > lastComma {
		decimal = lastDot
	} else if lastComma > -1 {
		decimal = lastComma
	}
	// A separator with three digits after it is a thousands separator, not a
	// decimal point: "1.500" is fifteen hundred, not one and a half.
	if decimal > -1 && len(cleaned)-decimal-1 > 2 {
		decimal = -1
	}

	var b strings.Builder
	for i, c := range cleaned {
		switch {
		case c == '-' && i == 0:
			b.WriteRune(c)
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case i == decimal:
			b.WriteRune('.')
		}
	}

	value, err := strconv.ParseFloat(b.String(), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", raw)
	}
	return value, nil
}

// Bool reads the several things people write for yes.
func (r CSVRow) Bool(column string) bool {
	switch strings.ToLower(r.Get(column)) {
	case "1", "true", "yes", "y", "ya", "aktif", "active":
		return true
	}
	return false
}

// ReadCSV parses an uploaded file into named rows.
//
// The header is lower-cased and trimmed, so "SKU ", "sku" and "Sku" are the
// same column — which they are, to everybody except a parser.
func ReadCSV(body []byte, required []string) ([]CSVRow, error) {
	// Excel writes a BOM; left in place it makes the first column's name
	// something no lookup will ever match.
	trimmed := strings.TrimPrefix(string(body), "\ufeff")

	reader := csv.NewReader(strings.NewReader(trimmed))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, Invalid("That file could not be read as CSV: %v", err)
	}
	if len(records) == 0 {
		return nil, Invalid("That file is empty.")
	}

	header := make([]string, len(records[0]))
	index := map[string]int{}
	for i, name := range records[0] {
		header[i] = strings.ToLower(strings.TrimSpace(name))
		index[header[i]] = i
	}
	for _, column := range required {
		if _, ok := index[strings.ToLower(column)]; !ok {
			return nil, Invalid("That file has no %q column. It needs: %s.",
				column, strings.Join(required, ", "))
		}
	}

	rows := make([]CSVRow, 0, len(records)-1)
	for line, record := range records[1:] {
		values := map[string]string{}
		for i, name := range header {
			if i < len(record) {
				values[name] = record[i]
			}
		}
		// A row of nothing is what a trailing newline looks like, and it
		// should not be reported as an error.
		empty := true
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		rows = append(rows, CSVRow{Line: line + 2, values: values})
	}
	return rows, nil
}

// ImportResult is what an upload would do, or did.
type ImportResult struct {
	// DryRun is the whole point: an import says what it would do before it
	// does it, because a three-hundred-row mistake is not something anybody
	// wants to discover afterwards.
	DryRun   bool          `json:"dryRun"`
	Rows     int           `json:"rows"`
	Created  int           `json:"created"`
	Updated  int           `json:"updated"`
	Skipped  int           `json:"skipped"`
	Failures []ImportError `json:"failures"`
}

// ImportError names the row, so somebody can find it in their spreadsheet.
type ImportError struct {
	Line    int    `json:"line"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// Fail records a row that could not be imported. The import carries on: a file
// with one bad line should not lose the other two hundred and ninety-nine.
func (r *ImportResult) Fail(line int, subject, message string) {
	r.Failures = append(r.Failures, ImportError{Line: line, Subject: subject, Message: message})
	r.Skipped++
}

// Body reads a raw upload, however it was sent.
//
// A file input posts multipart; a script posts the bytes. Both are the same
// spreadsheet, and requiring one shape would send somebody looking for a
// browser to do something curl could.
func Body(r *http.Request, limit int64) ([]byte, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(limit); err != nil {
			return nil, Invalid("That upload could not be read.")
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, Invalid("The upload needs a file field called \"file\".")
		}
		defer file.Close()
		return readAll(file, limit)
	}
	return readAll(r.Body, limit)
}

func readAll(r interface{ Read([]byte) (int, error) }, limit int64) ([]byte, error) {
	buf := make([]byte, 0, 64*1024)
	chunk := make([]byte, 32*1024)
	for int64(len(buf)) <= limit {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	if int64(len(buf)) > limit {
		return nil, Invalid("That file is too large.")
	}
	return buf, nil
}
