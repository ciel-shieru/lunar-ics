package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// Event represents a single prayer observance event.
type Event struct {
	Category    string // e.g. "GUANYIN_BIRTH", "NEW_MOON"
	SummaryEN   string // English summary for ICS SUMMARY field
	SummaryZH   string // Chinese name for display
	Description string // Full description with lunar date info
	LunarYear   int    // Chinese lunar year number
	LunarMonth  int    // lunar month (negative = leap month)
	LunarDay    int    // day within the lunar month
	GregDate    time.Time // The Gregorian date of this event
}

// Event category identifiers for sorting and deduplication.
const (
	GUANYIN_BIRTH         = "GUANYIN_BIRTH"
	GUANYIN_ENLIGHTENMENT = "GUANYIN_ENLIGHTENMENT"
	GUANYIN_RENUNCIATION  = "GUANYIN_RENUNCIATION"
	NEW_MOON_OBSERVANCE   = "NEW_MOON"
	FULL_MOON_OBSERVANCE  = "FULL_MOON"
	QINGMING_FESTIVAL     = "QINGMING"
)

// Guanyin commemoration dates: lunar month / day pairs.
var guanyinDates = []struct{ Month, Day int }{
	{2, 19}, // Birthday - 观世音菩萨圣诞
	{6, 19}, // Enlightenment - 观音菩萨成道
	{9, 19}, // Renunciation - 观世音菩萨出家
}

// Festival overrides for specific lunar 1st/15th days.
var festivalOverrides = map[string]struct{ EN, ZH string }{
	"1-1":   {"Lunar New Year (Spring Festival)", "春节"},
	"1-15":  {"Lantern Festival (Shangyuan)", "元宵节"},
	"7-15":  {"Ghost Festival (Zhongyuan)", "中元节"},
	"8-15":  {"Mid-Autumn Festival", "中秋节"},
}

// Chinese numerals for lunar month names.
var chineseMonths = []string{"", "正月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "十一月", "腊月"}

func ordinal(n int) string {
	switch n % 100 {
	case 11, 12, 13:
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}

// chineseNumeral converts a number to Chinese numeral characters.
func chineseNumeral(n int) string {
	digits := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	var result strings.Builder
	for _, c := range fmt.Sprintf("%d", n) {
		if c >= '0' && c <= '9' {
			result.WriteString(digits[c-'0'])
		}
	}
	return result.String()
}

// buildDescription creates the event description with lunar date info.
func buildDescription(lunarYear, month, day int) string {
	leapPrefix := ""
	if month < 0 {
		leapPrefix = "Leap "
		month = -month
	}
	return fmt.Sprintf("Lunar %s%d%s month, %dth day", leapPrefix, lunarYear, chineseMonths[month], day)
}

// GenerateEvents produces all prayer events for the given Gregorian years.
func GenerateEvents(years []int) ([]Event, error) {
	var events []Event

	for _, gregYear := range years {
		solar := calendar.NewSolar(gregYear, 6, 15, 0, 0, 0)
		lunarForYear := solar.GetLunar()
		lunarYear := lunarForYear.GetYear()

		lYear := calendar.NewLunarYear(lunarYear)
		leapMonth := lYear.GetLeapMonth()

		for _, m := range lunarMonths(leapMonth) {
			// Day 1 (new moon / shuo) - always present
			lunar := calendar.NewLunarFromYmd(lunarYear, abs(m), 1)
			solar := lunar.GetSolar()
			events = append(events, Event{
				Category:   NEW_MOON_OBSERVANCE,
				LunarYear:  lunarYear,
				LunarMonth: m,
				LunarDay:   1,
				GregDate:   time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC),
			})

			// Day 15 (full moon / wang) - always present for both normal and leap months
			lunar = calendar.NewLunarFromYmd(lunarYear, abs(m), 15)
			solar = lunar.GetSolar()
			events = append(events, Event{
				Category:   FULL_MOON_OBSERVANCE,
				LunarYear:  lunarYear,
				LunarMonth: m,
				LunarDay:   15,
				GregDate:   time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC),
			})
		}

		// Guanyin commemoration days (Category A) - these use the lunar year number
		for _, gd := range guanyinDates {
			lunar := calendar.NewLunarFromYmd(lunarYear, gd.Month, gd.Day)
			solar := lunar.GetSolar()

			var cat string
			switch {
			case gd.Month == 2 && gd.Day == 19:
				cat = GUANYIN_BIRTH
			case gd.Month == 6 && gd.Day == 19:
				cat = GUANYIN_ENLIGHTENMENT
			case gd.Month == 9 && gd.Day == 19:
				cat = GUANYIN_RENUNCIATION
			}

			events = append(events, Event{
				Category:   cat,
				LunarYear:  lunarYear,
				LunarMonth: gd.Month,
				LunarDay:   gd.Day,
				GregDate:   time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC),
			})
		}

		// Qingming Festival (solar term) - falls on April 4 or 5 each Gregorian year
		qingmingDate := findQingming(gregYear)
		events = append(events, Event{
			Category:   QINGMING_FESTIVAL,
			LunarYear:  lunarYear,
			LunarMonth: 0,
			LunarDay:   0,
			GregDate:   qingmingDate,
		})
	}

	// Now enrich events with summaries and descriptions.
	enriched := make([]Event, len(events))
	for i, e := range events {
		enriched[i] = enrichEvent(e)
	}

	// Sort by Gregorian date ascending, then by category priority.
	sort.Slice(enriched, func(i, j int) bool {
		if enriched[i].GregDate.Equal(enriched[j].GregDate) {
			return eventPriority(enriched[i]) < eventPriority(enriched[j])
		}
		return enriched[i].GregDate.Before(enriched[j].GregDate)
	})

	// Deduplicate: keep one event per (category, gregorian-date).
	seen := make(map[string]bool)
	var deduped []Event
	for _, e := range enriched {
		key := fmt.Sprintf("%s-%d-%d-%d", e.Category, e.GregDate.Year(), int(e.GregDate.Month()), e.GregDate.Day())
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, e)
		}
	}

	return deduped, nil
}

func enrichEvent(e Event) Event {
	monthAbs := abs(e.LunarMonth)
	lunarKey := fmt.Sprintf("%d-%d", monthAbs, e.LunarDay)

	if override, ok := festivalOverrides[lunarKey]; ok {
		e.SummaryEN = override.EN
		e.SummaryZH = override.ZH
		return e
	}

	switch e.Category {
	case GUANYIN_BIRTH:
		e.SummaryEN = "Guanyin Bodhisattva's Birthday"
		e.SummaryZH = "观世音菩萨圣诞"
	case GUANYIN_ENLIGHTENMENT:
		e.SummaryEN = "Guanyin Bodhisattva's Enlightenment"
		e.SummaryZH = "观音菩萨成道"
	case GUANYIN_RENUNCIATION:
		e.SummaryEN = "Guanyin Bodhisattva's Renunciation"
		e.SummaryZH = "观世音菩萨出家"
	case QINGMING_FESTIVAL:
		e.SummaryEN = "Qingming Festival (Tomb-Sweeping Day)"
		e.SummaryZH = "清明节"
		e.Description = fmt.Sprintf("Solar term Qingming (%s %d)", e.GregDate.Month(), e.GregDate.Day())
		return e
	case NEW_MOON_OBSERVANCE:
		leapPrefix := ""
		if e.LunarMonth < 0 {
			leapPrefix = "Leap "
		}
		e.SummaryEN = fmt.Sprintf("%sNew-Moon Observance Day", leapPrefix)
		e.SummaryZH = chineseNumeral(monthAbs) + "月" + chineseNumeral(e.LunarDay) + "日 (朔)"
	case FULL_MOON_OBSERVANCE:
		leapPrefix := ""
		if e.LunarMonth < 0 {
			leapPrefix = "Leap "
		}
		e.SummaryEN = fmt.Sprintf("%sFull-Moon Observance Day", leapPrefix)
		e.SummaryZH = chineseNumeral(monthAbs) + "月" + chineseNumeral(e.LunarDay) + "日 (望)"
	}

	// Build description.
	monthDisplay := abs(e.LunarMonth)
	leapStr := ""
	if e.LunarMonth < 0 {
		leapStr = "Leap "
	}
	e.Description = fmt.Sprintf("%s%s%d月%s日", leapStr, chineseMonths[monthDisplay], e.LunarYear, chineseNumeral(e.LunarDay))

	return e
}

func eventPriority(e Event) int {
	switch e.Category {
	case GUANYIN_BIRTH:
		return 1
	case GUANYIN_ENLIGHTENMENT:
		return 2
	case GUANYIN_RENUNCIATION:
		return 3
	case QINGMING_FESTIVAL:
		return 4
	default:
		return 5 // NEW_MOON and FULL_MOON have lower priority
	}
}

func lunarMonths(leapMonth int) []int {
	var months []int
	for m := 1; m <= 12; m++ {
		months = append(months, m)
		if leapMonth > 0 && m == leapMonth {
			months = append(months, -m)
		}
	}
	return months
}

func findQingming(year int) time.Time {
	for day := 3; day <= 7; day++ {
		solar := calendar.NewSolar(year, 4, day, 0, 0, 0)
		lunar := solar.GetLunar()
		if lunar.GetJieQi() == "清明" {
			return time.Date(solar.GetYear(), time.Month(solar.GetMonth()), solar.GetDay(), 0, 0, 0, 0, time.UTC)
		}
	}
	// Fallback to April 4 (Qingming always falls on Apr 4-6)
	return time.Date(year, 4, 4, 0, 0, 0, 0, time.UTC)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
