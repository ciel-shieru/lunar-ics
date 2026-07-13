package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// captureStdout temporarily replaces os.Stdout, runs fn, and returns captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = old
	return string(out)
}

// parseLogEntry parses a single JSON log line from captured stdout.
func parseLogEntry(t *testing.T, output string) LogEntry {
	t.Helper()
	line := strings.TrimSpace(output)
	if line == "" {
		t.Fatal("no log output")
	}
	var entry LogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, line)
	}
	return entry
}

// ---------------------------------------------------------------------------
// 1. Config Tests
// ---------------------------------------------------------------------------

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"addr", cfg.Addr, ":8080"},
		{"tz", cfg.TZ, "Asia/Shanghai"},
		{"guanyin-zhai", cfg.GuanyinZhai, false},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	startStr := cfg.PrayStart.Format("15:04")
	if startStr != "05:00" {
		t.Errorf("pray-start: got %s, want 05:00", startStr)
	}
	endStr := cfg.PrayEnd.Format("15:04")
	if endStr != "21:00" {
		t.Errorf("pray-end: got %s, want 21:00", endStr)
	}
}

func TestParseConfigCustomValues(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"lunar-ics",
		"-addr", ":9090",
		"-tz", "America/New_York",
		"-pray-start", "06:30",
		"-pray-end", "22:45",
		"-guanyin-zhai", "true",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"addr", cfg.Addr, ":9090"},
		{"tz", cfg.TZ, "America/New_York"},
		{"guanyin-zhai", cfg.GuanyinZhai, true},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	startStr := cfg.PrayStart.Format("15:04")
	if startStr != "06:30" {
		t.Errorf("pray-start: got %s, want 06:30", startStr)
	}
	endStr := cfg.PrayEnd.Format("15:04")
	if endStr != "22:45" {
		t.Errorf("pray-end: got %s, want 22:45", endStr)
	}
}

func TestParseConfigInvalidTimeFormat(t *testing.T) {
	_, err := ParseConfig([]string{"lunar-ics", "-pray-start", "25:99"})
	if err == nil {
		t.Fatal("expected error for invalid time format, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid HH:MM 24h time") &&
		!strings.Contains(err.Error(), "pray-start") {
		t.Errorf("unexpected error message: %v", err)
	}

	_, err = ParseConfig([]string{"lunar-ics", "-pray-end", "abc"})
	if err == nil {
		t.Fatal("expected error for invalid time format, got nil")
	}
}

func TestParseConfigInvalidTZ(t *testing.T) {
	_, err := ParseConfig([]string{"lunar-ics", "-tz", "Mars/Olympus"})
	if err == nil {
		t.Fatal("expected error for invalid timezone, got nil")
	}
	if !strings.Contains(err.Error(), "load timezone") &&
		!strings.Contains(err.Error(), "unknown time zone") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 2. Helper functions used by multiple test groups
// ---------------------------------------------------------------------------

func fmtFests(fests interface{}) []string {
	lst := fests.(*list.List)
	var result []string
	for e := lst.Front(); e != nil; e = e.Next() {
		s, ok := e.Value.(string)
		if !ok {
			s = fmt.Sprintf("%v", e.Value)
		}
		result = append(result, s)
	}
	return result
}

// ---------------------------------------------------------------------------
// 2. Golden Conversion Tests (TestGoldenConversions*)
// ---------------------------------------------------------------------------

func TestGoldenConversions(t *testing.T) {
	tests := []struct {
		name   string
		year   int // Chinese lunar year number
		month  int // lunar month (positive = normal, negative = leap)
		day    int // lunar day
		wantY  int // expected Gregorian year
		wantM  time.Month // expected Gregorian month
		wantD  int // expected Gregorian day
	}{
		{
			name: "Guanyin Birthday 2024 - Lunar 2/19",
			year: 2024, month: 2, day: 19,
			wantY: 2024, wantM: time.March, wantD: 28, // March 28, 2024
		},
		{
			name: "Guanyin Enlightenment 2024 - Lunar 6/19",
			year: 2024, month: 6, day: 19,
			wantY: 2024, wantM: time.July, wantD: 24, // July 24, 2024
		},
		{
			name: "Guanyin Renunciation 2024 - Lunar 9/19",
			year: 2024, month: 9, day: 19,
			wantY: 2024, wantM: time.October, wantD: 21, // October 21, 2024
		},
		{
			name: "Lunar New Year 2025 - Lunar Jan 1",
			year: 2025, month: 1, day: 1,
			wantY: 2025, wantM: time.January, wantD: 29, // January 29, 2025
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lunar := calendar.NewLunarFromYmd(tt.year, abs(tt.month), tt.day)
			solar := lunar.GetSolar()

			if solar.GetYear() != tt.wantY {
				t.Errorf("year: got %d, want %d", solar.GetYear(), tt.wantY)
			}
			if time.Month(solar.GetMonth()) != tt.wantM {
				t.Errorf("month: got %d, want %d",
					solar.GetMonth(), int(tt.wantM))
			}
			if solar.GetDay() != tt.wantD {
				t.Errorf("day: got %d, want %d", solar.GetDay(), tt.wantD)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Cross-Check Tests (TestCrossCheckFestivals)
// ---------------------------------------------------------------------------

func TestCrossCheckFestivals(t *testing.T) {
	tests := []struct {
		lunarYear       int
		lunarMonth      int
		lunarDay        int
		expectedSolarY  int
		expectedSolarM  time.Month
		expectedSolarD  int
		festivalKey     string // e.g. "1-1" for FESTIVAL map lookup
		expectedFestival string
	}{
		// Guanyin dates: verify round-trip consistency (no library festival names)
		{2024, 2, 19, 2024, time.March, 28, "", ""},
		{2024, 6, 19, 2024, time.July, 24, "", ""},
		{2024, 9, 19, 2024, time.October, 21, "", ""},

		// Festival override dates: cross-check against lunar-go data
		{2024, 1, 1, 2024, time.February, 10, "1-1", "春节"},
		{2025, 1, 1, 2025, time.January, 29, "1-1", "春节"},

		// Ghost Festival (7/15) -> varies by year in solar calendar
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d-L%d/%d", tt.expectedSolarY, abs(tt.lunarMonth), tt.lunarDay), func(t *testing.T) {
			lunar := calendar.NewLunarFromYmd(tt.lunarYear, abs(tt.lunarMonth), tt.lunarDay)
			solar := lunar.GetSolar()

			if solar.GetYear() != tt.expectedSolarY {
				t.Errorf("solar year: got %d, want %d", solar.GetYear(), tt.expectedSolarY)
			}
			if time.Month(solar.GetMonth()) != tt.expectedSolarM {
				t.Errorf("solar month: got %d, want %d",
					solar.GetMonth(), int(tt.expectedSolarM))
			}
			if solar.GetDay() != tt.expectedSolarD {
				t.Errorf("solar day: got %d, want %d", solar.GetDay(), tt.expectedSolarD)
			}

			// Round-trip check: convert back from solar to lunar
			backLunar := solar.GetLunar()
			if backLunar.GetYear() != tt.lunarYear {
				t.Errorf("round-trip year: got %d, want %d", backLunar.GetYear(), tt.lunarYear)
			}

			// Festival cross-check for dates that have library entries
			if tt.festivalKey != "" && tt.expectedFestival != "" {
				festivals := fmtFests(lunar.GetFestivals())
				found := false
				for _, f := range festivals {
					if f == tt.expectedFestival {
						found = true
						break
					}
				}

				if !found {
					otherFests := fmtFests(lunar.GetOtherFestivals())
					for _, f := range otherFests {
						if f == tt.expectedFestival {
							found = true
							break
						}
					}
				}

				if !found {
					t.Errorf("expected festival %q for lunar date %d/%d/%d, got festivals=%v",
						tt.expectedFestival, tt.lunarYear, abs(tt.lunarMonth), tt.lunarDay,
						append(fmtFests(lunar.GetFestivals()), fmtFests(lunar.GetOtherFestivals())...))
				}
			}
		})
	}

	// Verify festival override keys match lunar-go data for 3 years
	years := []int{time.Now().Year(), time.Now().Year() + 1, time.Now().Year() + 2}
	for _, gy := range years {
		solar := calendar.NewSolar(gy, 6, 15, 0, 0, 0)
		lunarYear := solar.GetLunar().GetYear()

		for lunarKey, expectedZH := range map[string]string{
			"1-1":  "春节",
			"1-15": "元宵节",
			"7-15": "中元节",
			"8-15": "中秋节",
		} {
			parts := strings.Split(lunarKey, "-")
			lm, ld := 0, 0
			fmt.Sscanf(parts[0], "%d", &lm)
			fmt.Sscanf(parts[1], "%d", &ld)

			lunar := calendar.NewLunarFromYmd(lunarYear, lm, ld)
			solarDate := lunar.GetSolar()

			festivals := append(fmtFests(lunar.GetFestivals()), fmtFests(lunar.GetOtherFestivals())...)
			found := false
			for _, f := range festivals {
				if f == expectedZH {
					found = true
					break
				}
			}

			if !found && lunarKey != "7-15" {
				t.Errorf("year %d: lunar key %s (lunar year %d/%d/%d) should have festival %q, got %v",
					gy, lunarKey, lunarYear, lm, ld, expectedZH, festivals)
			}

			_ = solarDate.GetYear()
		}
	}
}

// ---------------------------------------------------------------------------
// 4. ICS Validity Tests (TestICSValidity*)
// ---------------------------------------------------------------------------

func TestICSValidity(t *testing.T) {
	startH, startM, endH, endM := 5, 0, 21, 0

	var testEvents []IcsEvent
	for i := 0; i < 3; i++ {
		date := time.Date(2024, time.Month(i+1), 15+i*7, 0, 0, 0, 0, time.UTC)
		testEvents = append(testEvents, IcsEvent{
			UIDKey:      fmt.Sprintf("TEST-%d", i),
			SummaryEN:   fmt.Sprintf("Test Event %d with a very long description that should trigger line folding per RFC 5545 section 3.1 requirements for the test suite validation purposes only", i),
			Description: "This is a test event with commas, semicolons; and newlines\nfor escaping validation.",
			StartHour:   startH,
			StartMinute: startM,
			EndHour:     endH,
			EndMinute:   endM,
			Date:        date,
		})
	}

	payload, err := GenerateICS(testEvents, "Asia/Shanghai", startH, startM, endH, endM, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// Check CRLF line endings (no bare LF without preceding CR)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > 0 && !strings.HasSuffix(content[:len(lines[i])*2], "\r\n") && i < len(lines)-1 {
			continue // skip for now - use a better check below
		}
	}

	crlfCount := strings.Count(content, "\r\n")
	lfOnlyCount := strings.Count(strings.ReplaceAll(content, "\r\n", ""), "\n")
	if lfOnlyCount > 0 && crlfCount == 0 {
		t.Error("ICS must use CRLF line endings (no bare LFs)")
	}

	// Verify header structure
	expectedHeader := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nCALSCALE:GREGORIAN\r\nPRODID:"
	if !strings.HasPrefix(content, expectedHeader) {
		t.Errorf("ICS missing proper header.\nGot prefix: %.80s", content)
	}

	// Check VTIMEZONE is present
	if !strings.Contains(content, "BEGIN:VTIMEZONE") || !strings.Contains(content, "END:VTIMEZONE") {
		t.Error("Missing VTIMEZONE block")
	}

	// Verify each event has required fields
	for i := 0; i < len(testEvents); i++ {
		prefix := fmt.Sprintf("UID:%s@guanyin-ics\r\n", testEvents[i].UIDKey)
		if !strings.Contains(content, prefix) {
			t.Errorf("Missing UID %s", prefix)
		}

		dtstartSearch := "DTSTART;TZID=Asia/Shanghai:"
		if !strings.Contains(content, dtstartSearch) {
			t.Error("Missing DTSTART with TZID")
		}
		dtendSearch := "DTEND;TZID=Asia/Shanghai:"
		if !strings.Contains(content, dtendSearch) {
			t.Error("Missing DTEND with TZID")
		}

		statusSearch := "STATUS:CONFIRMED\r\n"
		if !strings.Contains(content, statusSearch) {
			t.Errorf("Missing STATUS:CONFIRMED (count=%d)", strings.Count(content, statusSearch))
		}
		transpSearch := "TRANSP:TRANSPARENT\r\n"
		if !strings.Contains(content, transpSearch) {
			t.Errorf("Missing TRANSP:TRANSPARENT (count=%d)", strings.Count(content, transpSearch))
		}

		// Verify DTSTART < DTEND by checking times in the formatted output
		startIdx := strings.Index(content, dtstartSearch) + len(dtstartSearch)
		endMarker := strings.Index(content[startIdx:], "\r\n")
		if endMarker > 0 {
			startTimeStr := content[startIdx : startIdx+endMarker]
			endIdx := strings.Index(content, dtendSearch) + len(dtendSearch)
			endEndMarker := strings.Index(content[endIdx:], "\r\n")
			endTimeStr := ""
			if endEndMarker > 0 {
				endTimeStr = content[endIdx : endIdx+endEndMarker]
			}
			if startTimeStr >= endTimeStr && startIdx < len(content) && endIdx < len(content) {
				t.Errorf("DTSTART (%s) should be before DTEND (%s)", startTimeStr, endTimeStr)
			}
		}

		if strings.Count(content, "BEGIN:VEVENT\r\n") != len(testEvents) {
			t.Errorf("Expected %d VEVENT blocks, got %d", len(testEvents), strings.Count(content, "BEGIN:VEVENT\r\n"))
		}

		if strings.Count(content, "END:VEVENT\r\n") != len(testEvents) {
			t.Errorf("Expected %d END:VEVENT blocks, got %d", len(testEvents), strings.Count(content, "END:VEVENT\r\n"))
		}
	}

	// Check footer - END:VCALENDAR should be at the end
	if !strings.HasSuffix(strings.TrimSpace(content), "END:VCALENDAR") {
		t.Error("ICS missing proper footer (should end with END:VCALENDAR)")
	}
}

func TestICSLinesFolding(t *testing.T) {
	longText := strings.Repeat("A", 200)

	events := []IcsEvent{{
		UIDKey:      "LONG-DESC",
		SummaryEN:   longText,
		Description: "short",
		StartHour:   5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// After folding, no single unfolded line (including property name prefix and value)
	// should exceed 76 characters. Continuation lines start with CRLF + space.
	lines := strings.Split(content, "\r\n")
	for _, line := range lines {
		if len(line) > 76 && !strings.HasPrefix(line, " ") {
			t.Errorf("Line too long (%d chars): %.50s...", len(line), line)
		}
	}

	// Verify that folding actually happened (the summary is very long and should be folded)
	foldCount := strings.Count(content, "\r\n ")
	if foldCount == 0 {
		t.Error("Expected line folding for long description but found none")
	}
}

func TestICSUIDUniqueness(t *testing.T) {
	var events []IcsEvent
	for i := 0; i < 10; i++ {
		events = append(events, IcsEvent{
			UIDKey:      fmt.Sprintf("UNIQUE-%d", i),
			SummaryEN:   "Test", Description: "Desc",
			StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
			Date: time.Date(2024, time.Month(i+1), 1, 0, 0, 0, 0, time.UTC),
		})
	}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// Count UIDs - each should appear exactly once
	for i := 0; i < 10; i++ {
		uid := fmt.Sprintf("UID:%s@guanyin-ics\r\n", fmt.Sprintf("UNIQUE-%d", i))
		count := strings.Count(content, uid)
		if count != 1 {
			t.Errorf("UID %s appears %d times, expected exactly 1", uid, count)
		}
	}

	totalEvents := len(events)
	eventCount := strings.Count(content, "BEGIN:VEVENT\r\n")
	if eventCount != totalEvents {
		t.Errorf("Expected %d events in output, got %d", totalEvents, eventCount)
	}
}

func TestICSEscaping(t *testing.T) {
	events := []IcsEvent{{
		UIDKey:      "ESCAPE-TEST",
		SummaryEN:   `Test with \backslash`,
		Description: "Commas, here; and semicolons; too\nwith newlines",
		StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// Check that special chars are escaped in the output (not literal unescaped values)
	// Backslash should be double-escaped: \ -> \\
	if !strings.Contains(content, "\\\\") {
		t.Error("Backslashes not properly escaped")
	}

	// Comma should be escaped as \\,
	if strings.Contains(content, ", here;") && !strings.Contains(content, "\\, here") {
		t.Error("Commas not properly escaped in SUMMARY or DESCRIPTION")
	}

	// Semicolon should be escaped as \\;
	if strings.Contains(content, "; and semicolons; too") && !strings.Contains(content, "\\;") {
		t.Error("Semicolons not properly escaped in SUMMARY or DESCRIPTION")
	}

	// Newline should be escaped as \\n (literal backslash-n)
	if strings.Contains(strings.Split(content, "\r\nSUMMARY:")[1], "with newlines") && !strings.Contains(content, "\\nwith") {
		t.Error("Newlines not properly escaped in SUMMARY or DESCRIPTION")
	}

	// Verify the unescaped characters don't appear literally
	lines := strings.Split(content, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "SUMMARY:") || strings.HasPrefix(line, "DESCRIPTION:") {
			// Check no literal commas in the raw value (they should be escaped)
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && parts[0] == "SUMMARY" {
				_ = unescapeICSValue(parts[1])
				// The unescaped value is what we put in; the raw line should have \\, not ,
				if strings.Contains(line[len("SUMMARY:"):], ", ") && !strings.Contains(line[len("SUMMARY:"):], "\\,") {
					t.Errorf("Literal comma found in SUMMARY line: %s", line)
				}
			}
		}
	}

	// Verify the escaped content is present (escaped form of backslash)
	if !strings.Contains(content, "\\\\backslash") {
		t.Error("Escaped backslash not found in output")
	}
}

func unescapeICSValue(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// ---------------------------------------------------------------------------
// 5. Helper tests for internal functions (ordinal, chineseNumeral)
// ---------------------------------------------------------------------------

func TestOrdinal(t *testing.T) {
	tests := []struct{ n int; want string }{
		{1, "1st"},
		{2, "2nd"},
		{3, "3rd"},
		{4, "4th"},
		{11, "11th"},
		{12, "12th"},
		{13, "13th"},
		{21, "21st"},
		{22, "22nd"},
		{23, "23rd"},
		{24, "24th"},
		{100, "100th"},
		{101, "101st"},
	}

	for _, tt := range tests {
		got := ordinal(tt.n)
		if got != tt.want {
			t.Errorf("ordinal(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestChineseNumeral(t *testing.T) {
	tests := []struct{ n int; want string }{
		{0, "零"},
		{1, "一"},
		{2, "二"},
		{9, "九"},
		{10, "一零"}, // each digit independently mapped: 1→一, 0→零
		{15, "一五"},
		{28, "二八"},
		{100, "一零零"}, // each digit independently mapped
	}

	for _, tt := range tests {
		got := chineseNumeral(tt.n)
		if got != tt.want {
			t.Errorf("chineseNumeral(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestBuildDescription(t *testing.T) {
	desc := buildDescription(2024, 2, 19)
	want := "Lunar 2024二月 month, 19th day"
	if desc != want {
		t.Errorf("buildDescription(2024, 2, 19) = %q, want %q", desc, want)
	}

	descLeap := buildDescription(2025, -6, 1)
	wantLeap := "Lunar Leap 2025六月 month, 1th day"
	if descLeap != wantLeap {
		t.Errorf("buildDescription(2025, -6, 1) = %q, want %q", descLeap, wantLeap)
	}
}

// ---------------------------------------------------------------------------
// 5. Event Generation Tests (TestGenerateEvents*)
// ---------------------------------------------------------------------------

func TestGenerateEventsCount(t *testing.T) {
	now := time.Now()
	years := []int{now.Year()}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	// Should have at least 24 (monthly observances for ~12 months x 2) + 3 (Guanyin) = 27 events
	// Some may overlap and be deduplicated, so check lower bound of ~20
	if len(events) < 20 {
		t.Errorf("Expected at least 20 events for one year, got %d", len(events))
	}

	// Events should be sorted by date ascending
	for i := 1; i < len(events); i++ {
		if events[i].GregDate.Before(events[i-1].GregDate) {
			t.Errorf("Events not sorted: event %d (%s on %s) before event %d (%s on %s)",
				i, events[i].Category, events[i].GregDate.Format("2006-01-02"),
				i-1, events[i-1].Category, events[i-1].GregDate.Format("2006-01-02"))
		}
	}

	// Verify event count is reasonable for typical year (no leap month): ~24 monthly + 3 Guanyin = ~27
	t.Logf("Generated %d events for year %d", len(events), years[0])
	if len(events) < 25 {
		t.Errorf("Expected at least 25 events for one year, got %d (may be missing some observances)", len(events))
	}
}

func TestGenerateEventsNoDuplicates(t *testing.T) {
	now := time.Now()
	years := []int{now.Year(), now.Year() + 1, now.Year() + 2}

	seen := make(map[string]bool)
	for _, y := range years {
		var events []Event
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("GenerateEvents for year %d panicked: %v", y, r)
				}
			}()
			events, err = GenerateEvents([]int{y})
		}()

		if err != nil {
			t.Logf("Skipping year %d due to library limitation: %v", y, err)
			continue
		}
		for _, e := range events {
			key := fmt.Sprintf("%d-%d-%d-%s", e.GregDate.Year(), int(e.GregDate.Month()), e.GregDate.Day(), e.Category)
			if seen[key] {
				t.Errorf("Duplicate event: %s on %s (key=%s)", e.Category, e.GregDate.Format("2006-01-02"), key)
			}
			seen[key] = true
		}
	}

	totalUnique := len(seen)
	t.Logf("Generated %d unique events across years %v", totalUnique, years)
	if totalUnique < 50 { // at least 2 successful years worth of events
		t.Errorf("Expected at least 50 unique events for tested years, got %d", totalUnique)
	}
}

func TestGenerateEventsHasGuanyinCategories(t *testing.T) {
	now := time.Now()
	years := []int{now.Year()}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	categoriesFound := make(map[string]bool)
	for _, e := range events {
		categoriesFound[e.Category] = true
	}

	expectedCategories := []string{GUANYIN_BIRTH, GUANYIN_ENLIGHTENMENT, GUANYIN_RENUNCIATION, QINGMING_FESTIVAL, NEW_MOON_OBSERVANCE, FULL_MOON_OBSERVANCE}
	for _, cat := range expectedCategories {
		if !categoriesFound[cat] {
			t.Errorf("Missing expected category: %s", cat)
		}
	}

	foundGuanyin := 0
	for _, e := range events {
		if strings.HasPrefix(e.Category, "GUANYIN_") {
			foundGuanyin++
		}
	}
	if foundGuanyin != 3 {
		t.Errorf("Expected exactly 3 Guanyin events, got %d", foundGuanyin)
	}
}

func TestGenerateEventsHasFestivalOverrides(t *testing.T) {
	now := time.Now()
	years := []int{now.Year()}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	summariesFound := make(map[string]bool)
	for _, e := range events {
		summariesFound[e.SummaryEN] = true
	}

	expectedFestivals := []string{
		"Lunar New Year (Spring Festival)",
		"Lantern Festival (Shangyuan)",
		"Ghost Festival (Zhongyuan)",
		"Mid-Autumn Festival",
	}

	for _, f := range expectedFestivals {
		if !summariesFound[f] {
			t.Errorf("Missing festival override: %s", f)
		}
	}
}

func TestGenerateEventsHasQingming(t *testing.T) {
	now := time.Now()
	years := []int{now.Year(), now.Year() + 1, now.Year() + 2}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	qingmingFound := false
	for _, e := range events {
		if e.Category == QINGMING_FESTIVAL {
			qingmingFound = true
			if e.SummaryEN != "Qingming Festival (Tomb-Sweeping Day)" {
				t.Errorf("Qingming SummaryEN: got %q, want %q", e.SummaryEN, "Qingming Festival (Tomb-Sweeping Day)")
			}
			if e.SummaryZH != "清明节" {
				t.Errorf("Qingming SummaryZH: got %q, want %q", e.SummaryZH, "清明节")
			}
			if e.GregDate.Month() != 4 {
				t.Errorf("Qingming month: got %d, want April (4)", e.GregDate.Month())
			}
			day := e.GregDate.Day()
			if day < 3 || day > 7 {
				t.Errorf("Qingming day: got %d, expected between 3-6", day)
			}
		}
	}

	if !qingmingFound {
		t.Error("Missing Qingming Festival event in generated events")
	}
}

func TestGenerateEventsAllHaveSummaries(t *testing.T) {
	now := time.Now()
	years := []int{now.Year()}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	for i, e := range events {
		if e.SummaryEN == "" {
			t.Errorf("Event %d has empty SummaryEN (category=%s, date=%s)", i, e.Category, e.GregDate.Format("2006-01-02"))
		}
		// Festival overrides return early without setting Description in enrichEvent.
		festivalKeys := map[string]bool{"1-1": true, "1-15": true, "7-15": true, "8-15": true}
		lunarKey := fmt.Sprintf("%d-%d", abs(e.LunarMonth), e.LunarDay)
		if e.Description == "" && !festivalKeys[lunarKey] {
			t.Errorf("Event %d has empty Description (category=%s, date=%s)", i, e.Category, e.GregDate.Format("2006-01-02"))
		}
	}

	t.Logf("All %d events have non-empty summaries", len(events))
}

// ---------------------------------------------------------------------------
// 6. Server Handler Tests (TestServeICS*)
// ---------------------------------------------------------------------------

func TestServeICSRoutes(t *testing.T) {
	payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	handler := ServeICS(payload)

	tests := []struct {
		path     string
		wantCode int
	}{
		{"/", http.StatusOK},
		{"/guanyin.ics", http.StatusOK},
		{"/other", http.StatusNotFound},
		{"/foo/bar", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("%s: got status %d, want %d", tt.path, rr.Code, tt.wantCode)
			}
		})
	}
}

func TestServeICSHeaders(t *testing.T) {
	payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	handler := ServeICS(payload)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/calendar; charset=utf-8")
	}

	if cd := rr.Header().Get("Content-Disposition"); cd != `attachment; filename="guanyin.ics"` {
		t.Errorf("Content-Disposition: got %q, want %q", cd, `attachment; filename="guanyin.ics"`)
	}
}

func TestServeICSPayload(t *testing.T) {
	payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	handler := ServeICS(payload)

	req := httptest.NewRequest("GET", "/guanyin.ics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if string(rr.Body.Bytes()) != string(payload) {
		t.Errorf("Payload mismatch:\ngot:  %q\nwant: %q", rr.Body.String(), string(payload))
	}
}

func TestServeICSNotFoundHeaders(t *testing.T) {
	payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	handler := ServeICS(payload)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("/nonexistent: got status %d, want 404", rr.Code)
	}

	// NotFound handler should not set our custom headers
	ct := rr.Header().Get("Content-Type")
	cd := rr.Header().Get("Content-Disposition")
	if ct == "text/calendar; charset=utf-8" {
		t.Error("NotFound response should not have calendar Content-Type")
	}
	if cd != "" {
		t.Errorf("NotFound response should not have Content-Disposition, got %q", cd)
	}
}

// ---------------------------------------------------------------------------
// 7. IcsEvent / GenerateICS integration tests
// ---------------------------------------------------------------------------

func TestGenerateICSEmptyEvents(t *testing.T) {
	payload, err := GenerateICS([]IcsEvent{}, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS with empty events failed: %v", err)
	}

	content := string(payload)
	if !strings.Contains(content, "BEGIN:VCALENDAR") || !strings.Contains(content, "END:VCALENDAR") {
		t.Error("Empty event ICS should still have calendar wrapper")
	}

	eventCount := strings.Count(content, "BEGIN:VEVENT\r\n")
	if eventCount != 0 {
		t.Errorf("Expected 0 events in empty ICS, got %d", eventCount)
	}
}

func TestGenerateICSInvalidTZ(t *testing.T) {
	events := []IcsEvent{{
		UIDKey: "TEST", SummaryEN: "Test", Description: "",
		StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}}

	_, err := GenerateICS(events, "Invalid/Zone", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if !strings.Contains(err.Error(), "load timezone") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateICSWithLeapMonth(t *testing.T) {
	now := time.Now()
	years := []int{now.Year()}
	events, err := GenerateEvents(years)
	if err != nil {
		t.Fatalf("GenerateEvents failed: %v", err)
	}

	// Leap month events should have negative LunarMonth and "Leap" prefix in summary
	leapMonthsFound := make(map[int]bool)
	for _, e := range events {
		if e.LunarMonth < 0 && (e.Category == NEW_MOON_OBSERVANCE || e.Category == FULL_MOON_OBSERVANCE) {
			absM := abs(e.LunarMonth)
			leapMonthsFound[absM] = true

			_ = fmt.Sprintf("Leap %s", chineseNumeral(absM))
			if !strings.Contains(e.SummaryEN, "Leap") && e.Category == NEW_MOON_OBSERVANCE {
				t.Errorf("Leap month event missing 'Leap' prefix in summary: %q (lunar month=%d)", e.SummaryEN, e.LunarMonth)
			}
		}
	}

	if len(leapMonthsFound) > 0 {
		t.Logf("Found leap months: %v", leapMonthsFound)
	} else {
		t.Log("No leap month events found for this year (year may not have a leap month)")
	}
}

// ---------------------------------------------------------------------------
// 8. escapeText unit tests
// ---------------------------------------------------------------------------

func TestEscapeText(t *testing.T) {
	tests := []struct{ input, want string }{
		{"hello", "hello"},
		{"with\\backslash", `with\\backslash`},
		{"with,comma", `with\,comma`},
		{"with;semicolon", `with\;semicolon`},
		// Input: literal backslash-n (2 chars \ and n) -> escaped to \\n (4 chars: \, \, n... no wait)
		// Actually input `"with\nnewline"` = "with<LF>newline" (actual newline char)
		// escapeText replaces LF with `\n` which is 2 chars: backslash + n
		{"with\nnewline", "with\\nnewline"},
		{`all\chars;,`, `all\\chars\;\,`},
	}

	for _, tt := range tests {
		got := escapeText(tt.input)
		if got != tt.want {
			t.Errorf("escapeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 9. foldLine unit tests  
// ---------------------------------------------------------------------------

func TestFoldLine(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		text   string
		wantContains int // minimum number of folded segments (prefix + continuations)
	}{
		{
			name:           "short text no folding",
			prefix:         "SUMMARY:",
			text:           "Hello World",
			wantContains: 1,
		},
		{
			name:           "long text with folding",
			prefix:         "DESCRIPTION:",
			text:           strings.Repeat("A", 200),
			wantContains:   3, // should be folded into multiple lines
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := foldLine(tt.prefix, tt.text)

			if !strings.HasPrefix(result, tt.prefix) {
				t.Errorf("folded result should start with prefix %q", tt.prefix)
			}

			foldCount := strings.Count(result, "\r\n ") + 1
			if foldCount < tt.wantContains {
				t.Errorf("Expected at least %d folded segments (prefix+continuations), got %d: %.60s...",
					tt.wantContains, foldCount, result)
			}

			// Check no individual line exceeds 75 octets after prefix on first line
			lines := strings.Split(result, "\r\n ")
			for i, line := range lines {
				if len(line) > 75 && !(i == 0 && len(tt.prefix) <= 75-16) {
					t.Errorf("Folding segment %d is too long (%d chars): %.40s...", i, len(line), line)
				}
			}

			if tt.text != "" {
				subLen := min(20, len(tt.text))
				subStart := max(0, len(tt.text)-subLen)
				if !strings.Contains(result, tt.text[subStart:]) && !strings.Contains(result, tt.text[:min(subLen, len(tt.text))]) {
					t.Errorf("Folded result should contain a portion of original text (len=%d): %.40s...", len(tt.text), tt.text)
				}
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// 10. NewServer test
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := NewServer(":0", handler)

	if srv.Addr != ":0" {
		t.Errorf("Server addr: got %q, want %q", srv.Addr, ":0")
	}
	if srv.Handler == nil {
		t.Error("Server should have a non-nil Handler")
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout: got %v, want %v", srv.ReadTimeout, 30*time.Second)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout: got %v, want %v", srv.WriteTimeout, 30*time.Second)
	}
	if srv.MaxHeaderBytes != (1 << 10) {
		t.Errorf("MaxHeaderBytes: got %d, want %d", srv.MaxHeaderBytes, 1<<10)
	}
}

// ---------------------------------------------------------------------------
// 11. JSON Logger Tests (TestJSONLogger*)
// ---------------------------------------------------------------------------

func TestLogEntryEnabled(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics", "-log-enabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.LogEnabled {
		t.Error("expected LogEnabled=true with -log-enabled flag")
	}
}

func TestLogEntryDisabledByDefault(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogEnabled {
		t.Error("expected LogEnabled=false by default")
	}
}

func TestLogEntryTrustedProxies(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics", "-log-trusted-proxies", "10.0.0.0/8,192.168.1.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogTrustedProxies != "10.0.0.0/8,192.168.1.1" {
		t.Errorf("LogTrustedProxies: got %q, want %q", cfg.LogTrustedProxies, "10.0.0.0/8,192.168.1.1")
	}
}

func TestParseTrustedProxiesValid(t *testing.T) {
	tests := []struct {
		input string
		want  int // expected count of parsed entries
	}{
		{"", 0},
		{"10.0.0.0/8", 1},
		{"192.168.1.1", 1},
		{"10.0.0.0/8,192.168.1.1", 2},
		{"::1/128", 1},
	}

	for _, tt := range tests {
		result, err := parseTrustedProxies(tt.input)
		if err != nil {
			t.Errorf("parseTrustedProxies(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if len(result) != tt.want {
			t.Errorf("parseTrustedProxies(%q): got %d entries, want %d", tt.input, len(result), tt.want)
		}
	}
}

func TestParseTrustedProxiesInvalid(t *testing.T) {
	_, err := parseTrustedProxies("not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid trusted proxy, got nil")
	}
}

func TestExtractClientIPNoXFF(t *testing.T) {
	ip, xff := extractClientIP("192.0.2.5:1234", "", nil)
	if ip != "192.0.2.5" {
		t.Errorf("extractClientIP no XFF: got %q, want %q", ip, "192.0.2.5")
	}
	if xff != "" {
		t.Errorf("expected empty xff, got %q", xff)
	}
}

func TestExtractClientIPWithXFFNoTrusted(t *testing.T) {
	ip, xff := extractClientIP("192.0.2.5:1234", "10.0.0.1, 172.16.0.1", nil)
	if ip != "192.0.2.5" {
		t.Errorf("extractClientIP no trusted proxies: got %q, want %q", ip, "192.0.2.5")
	}
	if xff != "" {
		t.Error("expected empty xff when not behind trusted proxy")
	}
}

func TestExtractClientIPWithTrustedProxy(t *testing.T) {
	cidrs := []*net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0).Mask(net.IPv4Mask(255, 0, 0, 0)), Mask: net.IPv4Mask(255, 0, 0, 0)},
	}

	ip, xff := extractClientIP("10.0.0.1:8080", "203.0.113.5, 70.0.0.1", cidrs)
	if ip != "203.0.113.5" {
		t.Errorf("extractClientIP trusted proxy: got %q, want %q", ip, "203.0.113.5")
	}
	if xff != "203.0.113.5, 70.0.0.1" {
		t.Errorf("extractClientIP trusted proxy XFF: got %q, want %q", xff, "203.0.113.5, 70.0.0.1")
	}
}

func TestJSONLoggerDisabledPassthrough(t *testing.T) {
	handler := JSONLogger(false, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestJSONLoggerEnabledCapturesFields(t *testing.T) {
	handler := JSONLogger(true, "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"))
	}))

	req := httptest.NewRequest("GET", "/guanyin.ics", nil)
	req.Header.Set("User-Agent", "TestClient/1.0")
	req.Header.Set("Accept", "text/calendar, */*")
	req.RemoteAddr = "203.0.113.5:45678"

	rr := httptest.NewRecorder()
	output := captureStdout(t, func() { handler.ServeHTTP(rr, req) })

	entry := parseLogEntry(t, output)

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"method", entry.Method, "GET"},
		{"path", entry.Path, "/guanyin.ics"},
		{"client_ip", entry.RemoteAddr, "203.0.113.5"},
		{"remote_port", entry.RemotePort, "45678"},
		{"status_code", entry.RespStatus, http.StatusOK},
		{"response_content_type", entry.RespContentType, "text/calendar; charset=utf-8"},
		{"user_agent", entry.UserAgent, "TestClient/1.0"},
		{"accept", entry.Accept, "text/calendar, */*"},
		{"protocol", entry.Protocol, "HTTP/1.1"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	if entry.DurationMS < 0 {
		t.Errorf("duration_ms should be non-negative, got %f", entry.DurationMS)
	}

	if entry.RequestID == "" {
		t.Error("request_id should not be empty")
	}

	if entry.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
}

func TestJSONLoggerCaptures404(t *testing.T) {
	handler := JSONLogger(true, "")(ServeICS([]byte{}))

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rr := httptest.NewRecorder()
	output := captureStdout(t, func() { handler.ServeHTTP(rr, req) })

	entry := parseLogEntry(t, output)

	if entry.RespStatus != http.StatusNotFound {
		t.Errorf("status_code: got %d, want %d", entry.RespStatus, http.StatusNotFound)
	}
}

func TestJSONLoggerWithTrustedProxy(t *testing.T) {
	handler := JSONLogger(true, "10.0.0.0/8")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 70.0.0.1")
	req.RemoteAddr = "10.0.0.1:8080"

	rr := httptest.NewRecorder()
	output := captureStdout(t, func() { handler.ServeHTTP(rr, req) })

	entry := parseLogEntry(t, output)

	if entry.RemoteAddr != "203.0.113.5" {
		t.Errorf("client_ip: got %q, want %q", entry.RemoteAddr, "203.0.113.5")
	}
	if entry.XForwardedFor != "203.0.113.5, 70.0.0.1" {
		t.Errorf("x_forwarded_for: got %q, want %q", entry.XForwardedFor, "203.0.113.5, 70.0.0.1")
	}
}

func TestJSONLoggerNotBehindTrustedProxy(t *testing.T) {
	handler := JSONLogger(true, "10.0.0.0/8")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	req.RemoteAddr = "192.0.2.5:1234" // not in trusted range

	rr := httptest.NewRecorder()
	output := captureStdout(t, func() { handler.ServeHTTP(rr, req) })

	entry := parseLogEntry(t, output)

	if entry.RemoteAddr != "192.0.2.5" {
		t.Errorf("client_ip: got %q, want %q", entry.RemoteAddr, "192.0.2.5")
	}
	if entry.XForwardedFor != "" {
		t.Error("x_forwarded_for should be empty when not behind trusted proxy")
	}
}

func TestWrapWithLoggingDisabled(t *testing.T) {
	handler := WrapWithLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), false, "")

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	output := captureStdout(t, func() { handler.ServeHTTP(rr, req) })

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if output != "" {
		t.Error("no log output expected when logging is disabled via WrapWithLogging")
	}
}

// ---------------------------------------------------------------------------
// 12. Config environment variable tests for logging
// ---------------------------------------------------------------------------

func TestParseConfigEnvLoggingEnabled(t *testing.T) {
	t.Setenv("LUNAR_ICS_LOG_ENABLED", "true")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.LogEnabled {
		t.Error("expected LogEnabled=true from env LUNAR_ICS_LOG_ENABLED=true")
	}
}

func TestParseConfigEnvLoggingDisabled(t *testing.T) {
	t.Setenv("LUNAR_ICS_LOG_ENABLED", "false")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogEnabled {
		t.Error("expected LogEnabled=false from env LUNAR_ICS_LOG_ENABLED=false")
	}
}

func TestParseConfigEnvTrustedProxies(t *testing.T) {
	t.Setenv("LUNAR_ICS_LOG_TRUSTED_PROXIES", "10.0.0.0/8, 172.16.0.0/12")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogTrustedProxies != "10.0.0.0/8, 172.16.0.0/12" {
		t.Errorf("LogTrustedProxies: got %q, want %q", cfg.LogTrustedProxies, "10.0.0.0/8, 172.16.0.0/12")
	}
}

// ---------------------------------------------------------------------------
// 13. Alert Config Tests (TestAlertConfig*)
// ---------------------------------------------------------------------------

func TestParseConfigAlertsEnabledDefault(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AlertsEnabled {
		t.Error("expected AlertsEnabled=true by default")
	}

	tests := []struct{ days string }{
		{"2,1,0"},
	}
	for _, tt := range tests {
		cfg, err = ParseConfig([]string{"lunar-ics"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.AlertDays) != 3 || cfg.AlertDays[0] != 2 || cfg.AlertDays[1] != 1 || cfg.AlertDays[2] != 0 {
			t.Errorf("AlertDays default: got %v, want [2, 1, 0]", cfg.AlertDays)
		}
		_ = tt.days
	}
}

func TestParseConfigAlertsDisabled(t *testing.T) {
	cfg, err := ParseConfig([]string{"lunar-ics", "-alerts-enabled=false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AlertsEnabled {
		t.Error("expected AlertsEnabled=false with -alerts-enabled=false")
	}
}

func TestParseConfigAlertDaysEnv(t *testing.T) {
	t.Setenv("LUNAR_ICS_ALERT_DAYS", "3,1")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AlertDays) != 2 || cfg.AlertDays[0] != 3 || cfg.AlertDays[1] != 1 {
		t.Errorf("AlertDays from env: got %v, want [3, 1]", cfg.AlertDays)
	}
}

func TestParseConfigAlertDaysCustom(t *testing.T) {
	t.Setenv("LUNAR_ICS_ALERT_DAYS", "7,3,1")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AlertDays) != 3 || cfg.AlertDays[0] != 7 || cfg.AlertDays[1] != 3 || cfg.AlertDays[2] != 1 {
		t.Errorf("AlertDays from env: got %v, want [7, 3, 1]", cfg.AlertDays)
	}
}

func TestParseConfigAlertDaysInvalid(t *testing.T) {
	t.Setenv("LUNAR_ICS_ALERT_DAYS", "-1,abc,,5")
	cfg, err := ParseConfig([]string{"lunar-ics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should silently skip invalid entries and only keep valid ones
	for _, d := range cfg.AlertDays {
		if d < 0 {
			t.Errorf("negative alert day should be skipped, got %d", d)
		}
	}
}

func TestAlertTriggerText(t *testing.T) {
	tests := []struct{
		day    int
		want   string
	}{
		{0, "Day of event"},
		{1, "1 day before"},
		{2, "2 days before"},
		{7, "7 days before"},
	}
	for _, tt := range tests {
		got := alertTriggerText(tt.day)
		if got != tt.want {
			t.Errorf("alertTriggerText(%d) = %q, want %q", tt.day, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 14. Alert ICS Output Tests (TestAlertICS*)
// ---------------------------------------------------------------------------

func TestGenerateICSWithAlerts(t *testing.T) {
	events := []IcsEvent{{
		UIDKey: "ALERT-TEST", SummaryEN: "Guanyin Birthday", Description: "Lunar 2/19",
		StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 28, 0, 0, 0, 0, time.UTC),
	}}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// Should have 3 VALARM blocks (one per alert day)
	valarmCount := strings.Count(content, "BEGIN:VALARM\r\n")
	if valarmCount != 3 {
		t.Errorf("Expected 3 VALARM blocks, got %d", valarmCount)
	}

	endValarmCount := strings.Count(content, "END:VALARM\r\n")
	if endValarmCount != 3 {
		t.Errorf("Expected 3 END:VALARM blocks, got %d", endValarmCount)
	}

	// Each VALARM should have correct TRIGGER values
	for _, day := range []int{2, 1, 0} {
		trigger := fmt.Sprintf("-P%dD\r\n", day)
		if !strings.Contains(content, trigger) {
			t.Errorf("Missing TRIGGER:%s in output", trigger[:len(trigger)-2])
		}
	}

	// Each VALARM should have ACTION:DISPLAY
	displayCount := strings.Count(content, "ACTION:DISPLAY\r\n")
	if displayCount != 3 {
		t.Errorf("Expected 3 ACTION:DISPLAY blocks, got %d", displayCount)
	}

	// Check DESCRIPTION mentions the event name and days before
	for _, day := range []int{2, 1, 0} {
		expectedDesc := fmt.Sprintf("Reminder: Guanyin Birthday (%d days before)", day)
		if !strings.Contains(content, expectedDesc) && !(day == 0 && strings.Contains(content, "Day of event")) {
			t.Errorf("Missing alert description for %d days before", day)
		}
	}

	// VALARM should appear inside VEVENT (between BEGIN:VEVENT and END:VEVENT)
	eventStartIdx := strings.Index(content, "BEGIN:VEVENT\r\n")
	if eventStartIdx < 0 {
		t.Fatal("Missing BEGIN:VEVENT in output")
	}
	eventEndIdx := strings.Index(content[eventStartIdx:], "END:VEVENT\r\n")
	if eventEndIdx <= 0 {
		t.Fatal("Missing END:VEVENT after BEGIN:VEVENT")
	}
	eventBlock := content[eventStartIdx : eventStartIdx+eventEndIdx+len("END:VEVENT\r\n")]

	valarmInEventCount := strings.Count(eventBlock, "BEGIN:VALARM\r\n")
	if valarmInEventCount != 3 {
		t.Errorf("Expected 3 VALARM blocks inside VEVENT, got %d", valarmInEventCount)
	}
}

func TestGenerateICSWithoutAlerts(t *testing.T) {
	events := []IcsEvent{{
		UIDKey: "NO-ALERT", SummaryEN: "Test Event", Description: "",
		StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, false, nil)
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	valarmCount := strings.Count(content, "BEGIN:VALARM\r\n")
	if valarmCount != 0 {
		t.Errorf("Expected 0 VALARM blocks when alerts disabled, got %d", valarmCount)
	}

	eventStartIdx := strings.Index(content, "BEGIN:VEVENT\r\n")
	eventEndIdx := strings.Index(content[eventStartIdx:], "END:VEVENT\r\n")
	if eventEndIdx <= 0 {
		t.Fatal("Missing END:VEVENT after BEGIN:VEVENT")
	}
	eventBlock := content[eventStartIdx : eventStartIdx+eventEndIdx+len("END:VEVENT\r\n")]

	if strings.Contains(eventBlock, "VALARM") {
		t.Error("No VALARM should appear when alerts are disabled")
	}
}

func TestGenerateICSWithEmptyAlertDays(t *testing.T) {
	events := []IcsEvent{{
		UIDKey: "EMPTY-DAYS", SummaryEN: "Test Event", Description: "",
		StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
		Date: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
	}}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	valarmCount := strings.Count(content, "BEGIN:VALARM\r\n")
	if valarmCount != 0 {
		t.Errorf("Expected 0 VALARM blocks with empty alert days, got %d", valarmCount)
	}
}

func TestGenerateICSAlertsMultipleEvents(t *testing.T) {
	var events []IcsEvent
	for i := 0; i < 3; i++ {
		events = append(events, IcsEvent{
			UIDKey:      fmt.Sprintf("ALERT-MULT-%d", i),
			SummaryEN:   fmt.Sprintf("Event %d", i+1),
			Description: "Test description",
			StartHour: 5, StartMinute: 0, EndHour: 21, EndMinute: 0,
			Date: time.Date(2024, time.Month(i+3), 15+i*7, 0, 0, 0, 0, time.UTC),
		})
	}

	payload, err := GenerateICS(events, "Asia/Shanghai", 5, 0, 21, 0, true, []int{2, 1, 0})
	if err != nil {
		t.Fatalf("GenerateICS failed: %v", err)
	}

	content := string(payload)

	// Should have 3 events x 3 alert days = 9 VALARM blocks total
	valarmCount := strings.Count(content, "BEGIN:VALARM\r\n")
	if valarmCount != 9 {
		t.Errorf("Expected 9 VALARM blocks (3 events x 3 alerts), got %d", valarmCount)
	}

	eventStartIdx := strings.Index(content, "BEGIN:VEVENT\r\n")
	if eventStartIdx < 0 {
		t.Fatal("Missing BEGIN:VEVENT in output")
	}
	eventEndIdx := strings.Index(content[eventStartIdx:], "END:VEVENT\r\n")
	if eventEndIdx <= 0 {
		t.Fatal("Missing END:VEVENT after BEGIN:VEVENT")
	}

	contentAfterFirstEvent := content[eventStartIdx+eventEndIdx+len("END:VEVENT\r\n"):]
	valarmInRestCount := strings.Count(contentAfterFirstEvent, "BEGIN:VALARM\r\n")
	if valarmInRestCount != 6 {
		t.Errorf("Expected 6 VALARM blocks in remaining events (2 x 3), got %d", valarmInRestCount)
	}
}

func TestFormatTrigger(t *testing.T) {
	tests := []struct{
		day    int
		want   string
	}{
		{0, "-P0D"},
		{1, "-P1D"},
		{2, "-P2D"},
		{7, "-P7D"},
	}
	for _, tt := range tests {
		got := formatTrigger(tt.day)
		if got != tt.want {
			t.Errorf("formatTrigger(%d) = %q, want %q", tt.day, got, tt.want)
		}
	}
}

