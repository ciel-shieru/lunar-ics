package main

import (
	"flag"
	"fmt"
	"time"
)

type Config struct {
	Addr        string
	PrayStart   time.Time // zero-date, just the parsed time portion
	PrayEnd     time.Time // same format as PrayStart
	TZ          string    // IANA timezone name (e.g. "Asia/Shanghai")
	GuanyinZhai bool      // include optional vegetarian fast days
}

func ParseConfig(args []string) (*Config, error) {
	var cfg Config

	fs := flag.NewFlagSet("lunar-ics", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", ":8080", "HTTP server address")
	fs.StringVar(&cfg.TZ, "tz", "Asia/Shanghai", "IANA timezone name")
	fs.BoolVar(&cfg.GuanyinZhai, "guanyin-zhai", false, "opt-in for Guanyin vegetarian fast days")

	prayStart := fs.String("pray-start", "05:00", "start of prayer window (HH:MM)")
	prayEnd := fs.String("pray-end", "21:00", "end of prayer window (HH:MM)")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	startTime, err := parseTimeOnly(*prayStart)
	if err != nil {
		return nil, fmt.Errorf("validate pray-start: %w", err)
	}
	cfg.PrayStart = startTime

	endTime, err := parseTimeOnly(*prayEnd)
	if err != nil {
		return nil, fmt.Errorf("validate pray-end: %w", err)
	}
	cfg.PrayEnd = endTime

	_, err = time.LoadLocation(cfg.TZ)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", cfg.TZ, err)
	}

	return &cfg, nil
}

func parseTimeOnly(s string) (time.Time, error) {
	t, err := time.ParseInLocation("15:04", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a valid HH:MM 24h time", s)
	}
	return t, nil
}
