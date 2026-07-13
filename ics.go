package main

import (
	"fmt"
	"strings"
	"time"
)

// IcsEvent holds data for a single calendar event entry.
type IcsEvent struct {
	UIDKey      string
	SummaryEN   string
	Description string
	StartHour   int
	StartMinute int
	EndHour     int
	EndMinute   int
	Date        time.Time // Gregorian date, zero-time (year/month/day meaningful only)
}

// GenerateICS produces RFC 5545 compliant iCalendar output.
func GenerateICS(events []IcsEvent, tzName string, startH, startM, endH, endM int, alertsEnabled bool, alertDays []int) ([]byte, error) {
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", tzName, err)
	}

	var b strings.Builder

	// Header
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("PRODID:-//lunar-ics//Guanyin Prayer Calendar//EN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")

	// VTIMEZONE block
	_, offsetSeconds := time.Now().In(tz).Zone()
	offsetHours := offsetSeconds / 3600
	offsetMinutes := (offsetSeconds % 3600) / 60
	tzOffsetStr := fmt.Sprintf("%+03d%02d", offsetHours, offsetMinutes)

	b.WriteString("BEGIN:VTIMEZONE\r\n")
	b.WriteString(fmt.Sprintf("TZID:%s\r\n", tzName))
	b.WriteString("BEGIN:STANDARD\r\n")
	b.WriteString("DTSTART:19000101T000000\r\n")
	b.WriteString(fmt.Sprintf("TZOFFSETFROM:+0000\r\n"))
	b.WriteString(fmt.Sprintf("TZOFFSETTO:%s\r\n", tzOffsetStr))
	b.WriteString("END:STANDARD\r\n")
	b.WriteString("END:VTIMEZONE\r\n")

	// Events — apply the prayer window times (startH/startM/endH/endM) to each date.
	for _, event := range events {
		startTime := time.Date(
			event.Date.Year(), event.Date.Month(), event.Date.Day(),
			startH, startM, 0, 0, tz,
		)
		endTime := time.Date(
			event.Date.Year(), event.Date.Month(), event.Date.Day(),
			endH, endM, 0, 0, tz,
		)

		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString(fmt.Sprintf("UID:%s@guanyin-ics\r\n", escapeText(event.UIDKey)))
		b.WriteString(fmt.Sprintf("DTSTART;TZID=%s:%s\r\n", tzName, formatTime(startTime)))
		b.WriteString(fmt.Sprintf("DTEND;TZID=%s:%s\r\n", tzName, formatTime(endTime)))
		b.WriteString(foldLine("SUMMARY:", escapeText(event.SummaryEN)) + "\r\n")
		b.WriteString(foldLine("DESCRIPTION:", escapeText(event.Description)) + "\r\n")
		b.WriteString("STATUS:CONFIRMED\r\n")
		b.WriteString("TRANSP:TRANSPARENT\r\n")

		if alertsEnabled && len(alertDays) > 0 {
			for _, day := range alertDays {
				description := fmt.Sprintf("Reminder: %s (%d days before)", event.SummaryEN, day)
				trigger := formatTrigger(day)
				b.WriteString("BEGIN:VALARM\r\n")
				b.WriteString(fmt.Sprintf("TRIGGER:%s\r\n", trigger))
				b.WriteString("ACTION:DISPLAY\r\n")
				b.WriteString(foldLine("DESCRIPTION:", escapeText(description)) + "\r\n")
				b.WriteString("END:VALARM\r\n")
			}
		}

		b.WriteString("END:VEVENT\r\n")
	}

	// Footer
	b.WriteString("END:VCALENDAR\r\n")

	return []byte(b.String()), nil
}

// escapeText escapes special characters per RFC 5545 §3.3.11.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// formatTime formats a time as YYYYMMDDTHHmm00.
func formatTime(t time.Time) string {
	return t.Format("20060102T150405")
}

// formatTrigger produces a TRIGGER value for VALARM per RFC 5545 §3.8.6.2.
// A day of >= 0 means the alert fires that many days before the event start.
func formatTrigger(day int) string {
	return fmt.Sprintf("-P%dD", day)
}

// foldLine folds a text line to fit within 75 octets per RFC 5545 §3.1.
// The prefix (e.g. "SUMMARY:") is included in the first chunk; continuation
// lines start with CRLF + space.
func foldLine(prefix, text string) string {
	if len(text) == 0 {
		return prefix
	}

	maxLen := 75 - len(prefix)
	var parts []string

	pos := 0
	for pos < len(text) {
		end := pos + maxLen
		if end >= len(text) {
			parts = append(parts, text[pos:])
			break
		}
		parts = append(parts, text[pos:end])
		pos = end
	}

	return prefix + strings.Join(parts, "\r\n ")
}
