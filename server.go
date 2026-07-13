package main

import (
	"net/http"
	"strings"
	"time"
)

func ServeICS(payload []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimRight(r.URL.Path, "/")
		if path == "" {
			path = "/"
		}

		switch path {
		case "/", "/guanyin.ics":
			w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="guanyin.ics"`)
			w.WriteHeader(http.StatusOK)
			w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}
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
