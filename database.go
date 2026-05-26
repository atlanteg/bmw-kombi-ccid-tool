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

// CCIDEntry stores all language variants so the UI can search across all of them.
type CCIDEntry struct {
	ID          int
	Description string // primary display label: TITLE_ENGB, fallback TITLE_ENUS

	TitleENGB string
	LongENGB  string
	TitleENUS string
	LongENUS  string
	TitleDEDE string
	LongDEDE  string
}

// matchesQuery returns true when the CC-ID number or any language text contains q (case-insensitive).
func matchesQuery(e CCIDEntry, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strconv.Itoa(e.ID), q) {
		return true
	}
	for _, s := range []string{
		e.TitleENGB, e.LongENGB,
		e.TitleENUS, e.LongENUS,
		e.TitleDEDE, e.LongDEDE,
	} {
		if s != "" && strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
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
// Columns: CC_ID, WARN_LIGHT_IDENTIFIER,
//
//	TITLE_ENUS, LONGTEXT_ENUS, TITLE_DEDE, LONGTEXT_DEDE, TITLE_ENGB, LONGTEXT_ENGB
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
	cTitleGB := colIdx(col, "TITLE_ENGB")
	cLongGB := colIdx(col, "LONGTEXT_ENGB")
	cTitleUS := colIdx(col, "TITLE_ENUS")
	cLongUS := colIdx(col, "LONGTEXT_ENUS")
	cTitleDE := colIdx(col, "TITLE_DEDE")
	cLongDE := colIdx(col, "LONGTEXT_DEDE")

	clean := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "-" {
			return ""
		}
		return s
	}

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

		titleGB := clean(field(rec, cTitleGB))
		titleUS := clean(field(rec, cTitleUS))

		// Primary display label: prefer EN-GB, fallback to EN-US
		display := titleGB
		if display == "" {
			display = titleUS
		}

		entries = append(entries, CCIDEntry{
			ID:          id,
			Description: display,
			TitleENGB:   titleGB,
			LongENGB:    clean(field(rec, cLongGB)),
			TitleENUS:   titleUS,
			LongENUS:    clean(field(rec, cLongUS)),
			TitleDEDE:   clean(field(rec, cTitleDE)),
			LongDEDE:    clean(field(rec, cLongDE)),
		})
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
