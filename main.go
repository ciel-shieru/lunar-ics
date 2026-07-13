package main

import (
	"fmt"
	"log"
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

	currentYear := time.Now().Year()
	years := []int{currentYear, currentYear + 1, currentYear + 2}

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

	payload, err := GenerateICS(icsEvents, cfg.TZ, startH, startM, endH, endM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	handler := ServeICS(payload)
	srv := NewServer(cfg.Addr, handler)

	fmt.Printf("lunar-ics listening on %s with %d events\n", cfg.Addr, len(events))

	log.Fatal(srv.ListenAndServe())
}
