package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr              string
	PrayStart         time.Time // zero-date, just the parsed time portion
	PrayEnd           time.Time // same format as PrayStart
	TZ                string    // IANA timezone name (e.g. "Asia/Shanghai")
	GuanyinZhai       bool      // include optional vegetarian fast days
	YearsBefore       int       // number of years before current year to generate events for
	YearsAfter        int       // number of years after current year to generate events for
	LogEnabled        bool      // enable JSON request logging to stdout
	LogTrustedProxies string    // comma-separated list of IPs/CIDRs trusted as reverse proxies
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	s := os.Getenv(key)
	if s == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("env %q: %w", key, err)
	}
	return v, nil
}

func ParseConfig(args []string) (*Config, error) {
	var cfg Config

	addrDefault := envOr("LUNAR_ICS_ADDR", ":8080")
	tzDefault := envOr("LUNAR_ICS_TZ", "Asia/Shanghai")
	zhaiDefault := envOr("LUNAR_ICS_GUANYIN_ZHAIZAI", "false")
	logEnabledDefault := envOr("LUNAR_ICS_LOG_ENABLED", "false")
	logTrustedProxiesDefault := envOr("LUNAR_ICS_LOG_TRUSTED_PROXIES", "")

	var guanyinZhai bool
	if zhaiDefault == "true" || zhaiDefault == "1" {
		guanyinZhai = true
	}

	var logEnabled bool
	if logEnabledDefault == "true" || logEnabledDefault == "1" {
		logEnabled = true
	}

	fs := flag.NewFlagSet("lunar-ics", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", addrDefault, "HTTP server address")
	fs.StringVar(&cfg.TZ, "tz", tzDefault, "IANA timezone name (env: LUNAR_ICS_TZ)")
	fs.BoolVar(&cfg.GuanyinZhai, "guanyin-zhai", guanyinZhai, "opt-in for Guanyin vegetarian fast days (env: LUNAR_ICS_GUANYIN_ZHAIZAI)")
	fs.BoolVar(&cfg.LogEnabled, "log-enabled", logEnabled, "enable JSON request logging to stdout (env: LUNAR_ICS_LOG_ENABLED)")
	fs.StringVar(&cfg.LogTrustedProxies, "log-trusted-proxies", logTrustedProxiesDefault, "comma-separated list of trusted reverse proxy IPs/CIDRs for X-Forwarded-For parsing")

	yearsBeforeEnv, err := envInt("LUNAR_ICS_YEARS_BEFORE", 2)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	yearsAfterEnv, err := envInt("LUNAR_ICS_YEARS_AFTER", 2)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	yearsBefore := fs.Int("years-before", yearsBeforeEnv, "number of years before current year to generate events for (0 = none)")
	yearsAfter := fs.Int("years-after", yearsAfterEnv, "number of years after current year to generate events for (0 = none)")

	prayStartDefault := envOr("LUNAR_ICS_PRAY_START", "05:00")
	prayEndDefault := envOr("LUNAR_ICS_PRAY_END", "21:00")

	prayStart := fs.String("pray-start", prayStartDefault, "start of prayer window HH:MM (env: LUNAR_ICS_PRAY_START)")
	prayEnd := fs.String("pray-end", prayEndDefault, "end of prayer window HH:MM (env: LUNAR_ICS_PRAY_END)")

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

	cfg.YearsBefore = *yearsBefore
	cfg.YearsAfter = *yearsAfter

	if cfg.YearsBefore < 0 || cfg.YearsAfter < 0 {
		return nil, fmt.Errorf("years-before and years-after must be non-negative")
	}

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
