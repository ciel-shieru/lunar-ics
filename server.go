package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ServeICS returns an HTTP handler that serves ICS calendar data with optional dynamic alerts.
func ServeICS(basePayload []byte, alertsEnabled bool, alertDays []int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimRight(r.URL.Path, "/")
		if path == "" {
			path = "/"
		}

		switch path {
		case "/", "/guanyin.ics":
			w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="guanyin.ics"`)

			payload := basePayload
			if alertsEnabled {
				triggers := parseTriggers(r.URL.Query().Get("alert"))
				if len(triggers) == 0 && len(alertDays) > 0 {
					for _, day := range alertDays {
						triggers = append(triggers, formatTrigger(day))
					}
				}

				if len(triggers) > 0 {
					payload = insertAlertsForAllEvents(basePayload, triggers)
				}
			}

			w.WriteHeader(http.StatusOK)
			w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}
}

// insertAlertsForAllEvents inserts VALARM blocks into an ICS payload for every VEVENT.
func insertAlertsForAllEvents(baseICS []byte, triggers []string) []byte {
	content := string(baseICS)
	var b strings.Builder
	idx := 0

	for idx < len(content) {
		searchMarker := "TRANSP:TRANSPARENT\r\n"
		pos := strings.Index(content[idx:], searchMarker)
		if pos < 0 {
			b.WriteString(content[idx:])
			break
		}

		endOfMarker := idx + pos + len(searchMarker)
		b.WriteString(content[idx:endOfMarker])

		// Extract the event summary from this VEVENT for use in alert descriptions.
		summary := extractSummaryFromEvent(content, endOfMarker)

		for _, trigger := range triggers {
			description := fmt.Sprintf("Reminder: %s (%s before)", summary, trigger)
			b.WriteString("BEGIN:VALARM\r\n")
			b.WriteString(fmt.Sprintf("TRIGGER:%s\r\n", trigger))
			b.WriteString("ACTION:DISPLAY\r\n")
			b.WriteString(foldLine("DESCRIPTION:", description) + "\r\n")
			b.WriteString("END:VALARM\r\n")
		}

		idx = endOfMarker
	}

	return []byte(b.String())
}

// extractSummaryFromEvent finds the SUMMARY line within the VEVENT block that
// contains TRANSP:TRANSPARENT at startIdx, and returns its unescaped value.
func extractSummaryFromEvent(content string, startIdx int) string {
	// Find the beginning of this VEVENT by searching backwards for BEGIN:VEVENT.
	eventStart := strings.LastIndex(content[:startIdx], "BEGIN:VEVENT\r\n")
	if eventStart < 0 {
		return ""
	}
	eventStart += len("BEGIN:VEVENT\r\n")

	summMarker := "SUMMARY:"
	pos := strings.Index(content[eventStart:startIdx], summMarker)
	if pos < 0 {
		return ""
	}

	valPos := eventStart + pos + len(summMarker)
	// Handle line folding: continuation lines start with CRLF + space.
	for strings.HasPrefix(content[valPos:], "\r\n ") {
		valPos += 3
	}
	// Stop at the next \r\n (end of property value).
	eol := valPos + strings.Index(content[valPos:], "\r\n")
	if eol < 0 {
		return ""
	}

	return unescapeICSValue(content[valPos:eol])
}

func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 10, // 1KB
	}
}

// WrapWithLogging wraps a handler with the JSONLogger middleware if logging is enabled.
func WrapWithLogging(handler http.Handler, logEnabled bool, trustedProxies string) http.Handler {
	return JSONLogger(logEnabled, trustedProxies)(handler)
}
