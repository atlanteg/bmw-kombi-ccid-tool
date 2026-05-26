package main

import (
	"encoding/csv"
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

//go:embed cc_ids.csv
var embeddedCSV string

// CCIDEntry is one CC-ID with its short title and optional long description.
type CCIDEntry struct {
	ID          int
	Description string // TITLE_ENGB, fallback TITLE_ENUS
	LongText    string // LONGTEXT_ENGB (used for tooltips / detail view)
}

// loadAllEntries returns every CC-ID entry sorted by ID.
// Tries a cc_ids.csv next to the executable first; falls back to the embedded file.
func loadAllEntries() []CCIDEntry {
	return parseCSV(csvData())
}

// loadDescriptions is a convenience wrapper returning id→description map.
func loadDescriptions() map[int]string {
	entries := loadAllEntries()
	m := make(map[int]string, len(entries))
	for _, e := range entries {
		m[e.ID] = e.Description
	}
	return m
}

func csvData() string {
	execPath, err := os.Executable()
	if err != nil {
		return embeddedCSV
	}
	dir := filepath.Dir(execPath)

	// On macOS .app bundles the binary is inside Contents/MacOS/
	if runtime.GOOS == "darwin" {
		candidate := filepath.Join(dir, "..", "..", "..", "cc_ids.csv")
		if b, err := os.ReadFile(candidate); err == nil {
			return string(b)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "cc_ids.csv")); err == nil {
		return string(b)
	}
	return embeddedCSV
}

// parseCSV handles the multi-column BMW error code CSV.
// Columns: CC_ID, WARN_LIGHT_IDENTIFIER, TITLE_ENUS, LONGTEXT_ENUS,
//
//	TITLE_DEDE, LONGTEXT_DEDE, TITLE_ENGB, LONGTEXT_ENGB
func parseCSV(data string) []CCIDEntry {
	r := csv.NewReader(strings.NewReader(data))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil
	}

	col := make(map[string]int)
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}

	idCol := colIdx(col, "CC_ID")
	titleGB := colIdx(col, "TITLE_ENGB")
	titleUS := colIdx(col, "TITLE_ENUS")
	longGB := colIdx(col, "LONGTEXT_ENGB")

	var entries []CCIDEntry
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if idCol < 0 || idCol >= len(rec) {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(rec[idCol]))
		if err != nil {
			continue
		}

		title := field(rec, titleGB)
		if title == "" || title == "-" {
			title = field(rec, titleUS)
		}
		if title == "-" {
			title = ""
		}

		long := field(rec, longGB)
		if long == "-" {
			long = ""
		}

		entries = append(entries, CCIDEntry{ID: id, Description: title, LongText: long})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func colIdx(col map[string]int, name string) int {
	if i, ok := col[name]; ok {
		return i
	}
	return -1
}

func field(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[idx])
}
