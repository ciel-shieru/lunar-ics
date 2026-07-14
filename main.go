package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "time/tzdata"
)

func main() {
	cfg, err := ParseConfig(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	loc, _ := time.LoadLocation(cfg.TZ)
	now := time.Now().In(loc)
	currentYear := now.Year()

	var years []int
	for i := cfg.YearsBefore; i > 0; i-- {
		years = append(years, currentYear-i)
	}
	years = append(years, currentYear)
	for i := 1; i <= cfg.YearsAfter; i++ {
		years = append(years, currentYear+i)
	}

	events, err := GenerateEvents(years)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	startH := cfg.PrayStart.Hour()
	startM := cfg.PrayStart.Minute()
	endH := cfg.PrayEnd.Hour()
	endM := cfg.PrayEnd.Minute()

	var icsEvents []IcsEvent
	for _, e := range events {
		uidKey := fmt.Sprintf("%s-%d-%d-%d", e.Category, e.GregDate.Year(), int(e.GregDate.Month()), e.GregDate.Day())
		icsEvents = append(icsEvents, IcsEvent{
			UIDKey:      uidKey,
			SummaryEN:   e.SummaryEN,
			Description: e.Description,
			StartHour:   startH,
			StartMinute: startM,
			EndHour:     endH,
			EndMinute:   endM,
			Date:        e.GregDate,
		})
	}

	basePayload, err := GenerateICS(icsEvents, cfg.TZ, startH, startM, endH, endM, false, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var handler http.Handler = ServeICS(basePayload, cfg.AlertsEnabled, cfg.AlertDays)
	if cfg.LogEnabled {
		handler = WrapWithLogging(handler, cfg.LogEnabled, cfg.LogTrustedProxies)
	}
	srv := NewServer(cfg.Addr, handler)

	logPrefix := "lunar-ics"
	if cfg.LogEnabled {
		logPrefix += " (logging enabled)"
	}
	fmt.Printf("%s listening on %s with %d events\n", logPrefix, cfg.Addr, len(events))

	log.Fatal(srv.ListenAndServe())
}
