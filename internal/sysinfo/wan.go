package sysinfo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WAN looks up the public address.
type WAN struct {
	URL       string        // returns the address as plain text
	CacheFile string        // keeps the last answer between runs
	MaxAge    time.Duration // how long an answer is reused
	Timeout   time.Duration
	Now       func() time.Time
}

type wanCache struct {
	IP        string    `json:"ip"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Address returns the cached address while it's fresh, and looks it up
// otherwise. When the lookup fails it returns the last known address,
// or "".
func (w WAN) Address(ctx context.Context) string {
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	cached := w.read()
	if cached.IP != "" && now().Sub(cached.FetchedAt) < w.MaxAge {
		return cached.IP
	}
	ip, err := w.fetch(ctx)
	if err != nil {
		return cached.IP
	}
	w.write(wanCache{IP: ip, FetchedAt: now()})
	return ip
}

func (w WAN) fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(strings.TrimSpace(string(body)))
	if resp.StatusCode != http.StatusOK || ip == nil {
		return "", errBadAnswer
	}
	return ip.String(), nil
}

var errBadAnswer = errors.New("the WAN lookup didn't return an address")

func (w WAN) read() wanCache {
	var c wanCache
	if b, err := os.ReadFile(w.CacheFile); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func (w WAN) write(c wanCache) {
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(w.CacheFile), 0o755) == nil {
		_ = os.WriteFile(w.CacheFile, b, 0o644)
	}
}
