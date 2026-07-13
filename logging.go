package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// logStore holds the most recent LogEntry atomically.
var logStore atomic.Pointer[LogEntry]

// logMu serializes JSON marshaling and stdout writes for concurrent requests.
var logMu sync.Mutex

// LogEntry represents a single structured JSON log line for an HTTP request.
type LogEntry struct {
	Timestamp        string `json:"timestamp"`
	RequestID        string `json:"request_id"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	RemoteAddr       string `json:"remote_addr"`
	RemotePort       string `json:"remote_port"`
	ClientIP         string `json:"client_ip"`
	XForwardedFor    string `json:"x_forwarded_for,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	Accept           string `json:"accept,omitempty"`
	ContentLength    int64  `json:"content_length,omitempty"`
	Protocol         string `json:"protocol"`
	RespStatus       int    `json:"status_code"`
	RespContentType  string `json:"response_content_type,omitempty"`
	DurationMS       float64 `json:"duration_ms"`
}

// ResponseWriter wrapper that captures the status code written.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

// trustedProxy matches a single IP against a list of *net.IPNet.
func trustedProxy(ip net.IP, cidrs []*net.IPNet) bool {
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// parseTrustedProxies parses a comma-separated list of IPs or CIDRs into []*net.IPNet.
func parseTrustedProxies(s string) ([]*net.IPNet, error) {
	if s == "" {
		return nil, nil
	}
	var result []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Try parsing as CIDR first (e.g. "10.0.0.0/8" or "::1/128").
		_, ipNet, err := net.ParseCIDR(part)
		if err == nil {
			result = append(result, ipNet)
			continue
		}
		// Fall back to single IP (treat as /32 or /128).
		ip := net.ParseIP(part)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy: %q", part)
		}
		mask := net.IPv4Mask(255, 255, 255, 255)
		if ip.To4() == nil {
			mask = net.IPMask{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		}
		result = append(result, &net.IPNet{IP: ip.Mask(mask), Mask: mask})
	}
	return result, nil
}

// extractClientIP determines the real client IP from X-Forwarded-For when a
// trusted proxy is detected between us and the caller. Returns (clientIP string, xffHeaderOrEmpty).
func extractClientIP(remoteAddr string, xff string, trusted []*net.IPNet) (string, string) {
	if xff == "" || len(trusted) == 0 {
		ip := remoteAddr
		if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
			ip = h
		}
		return ip, ""
	}

	// Extract the IP portion from RemoteAddr to check if it's a trusted proxy.
	connIP := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		connIP = h
	}
	peerIP := net.ParseIP(connIP)
	if peerIP == nil || !trustedProxy(peerIP, trusted) {
		// Not behind a trusted proxy — log the raw RemoteAddr IP.
		return connIP, ""
	}

	// Behind a trusted proxy: parse X-Forwarded-For. The leftmost entry is the client.
	parts := strings.Split(xff, ",")
	clientIP := strings.TrimSpace(parts[0])
	if clientIP == "" {
		clientIP = connIP // fallback — shouldn't happen but be safe.
	}
	return clientIP, xff
}

// generateRequestID creates a 16-byte hex request ID.
func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%016x", b)
}

// JSONLogger returns an http.Handler middleware that stores each request log entry
// in a package-level atomic.Value. If enabled is false, it returns the next
// handler unchanged (zero overhead).
func JSONLogger(enabled bool, trustedProxies string) func(http.Handler) http.Handler {
	if !enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	cidrs, err := parseTrustedProxies(trustedProxies)
	if err != nil {
		panic(fmt.Sprintf("jsonlogger: invalid trusted-proxies: %v", err))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			reqID := generateRequestID()

			// Store request ID in a context key so downstream handlers can read it.
			ctx := r.Context()
			type reqIDKey struct{}
			ctx = context.WithValue(ctx, reqIDKey{}, reqID)

			next.ServeHTTP(rec, r.WithContext(ctx))

			duration := time.Since(start).Seconds() * 1000

			clientIP, xffHeader := extractClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), cidrs)
			respCT := rec.Header().Get("Content-Type")

			entry := LogEntry{
				Timestamp:     start.UTC().Format(time.RFC3339Nano),
				RequestID:     reqID,
				Method:        r.Method,
				Path:          r.URL.Path,
				RemoteAddr:    clientIP,
				UserAgent:     r.Header.Get("User-Agent"),
				Accept:        r.Header.Get("Accept"),
				ContentLength: r.ContentLength,
				Protocol:      r.Proto,
				RespStatus:    rec.statusCode,
				RespContentType: respCT,
				DurationMS:    duration,
			}

			if xffHeader != "" {
				entry.XForwardedFor = xffHeader
			}

			// Extract port from RemoteAddr.
			_, port, err := net.SplitHostPort(r.RemoteAddr)
			if err == nil {
				entry.RemotePort = port
			} else {
				entry.RemotePort = "0"
			}

			logStore.Store(&entry)

			// Emit the log entry as a single-line JSON to stdout.
			b, err := json.Marshal(entry)
			if err == nil {
				logMu.Lock()
				fmt.Fprintln(os.Stdout, string(b))
				logMu.Unlock()
			}
		})
	}
}

// GetLastLogEntry returns the most recent LogEntry stored by JSONLogger, or nil.
func GetLastLogEntry() *LogEntry {
	return logStore.Load()
}
