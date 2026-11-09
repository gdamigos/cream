// the opt-in usage check (usage_check in config.toml), asks Anthropic and OpenAI how much of a plan a login used
// the token only ever goes into the Authorization header, the cache holds percentages and times
package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gdamigos/cream/internal/fsutil"
)

const (
	ClaudeURL = "https://api.anthropic.com/api/oauth/usage"
	CodexURL  = "https://chatgpt.com/backend-api/wham/usage"
	TTL       = 5 * time.Minute
)

type Window struct {
	Name     string    `json:"name"` // 5h, 7d, 7d opus
	Used     float64   `json:"used"` // percent, 100 means the limit is reached
	ResetsAt time.Time `json:"resets_at,omitzero"`
}

type Result struct {
	Windows   []Window  `json:"windows"`
	Limited   bool      `json:"limited"`
	CheckedAt time.Time `json:"checked_at"`
}

// the highest use of any window
func (r *Result) Peak() float64 {
	var p float64
	for _, w := range r.Windows {
		p = max(p, w.Used)
	}
	return p
}

// when every full window has reset, zero when not limited or unknown
func (r *Result) Until() time.Time {
	var t time.Time
	for _, w := range r.Windows {
		if w.Used >= 100 && w.ResetsAt.After(t) {
			t = w.ResetsAt
		}
	}
	return t
}

// what a tool's login needs for its usage request
type Login struct {
	Adapter   string // claude or codex
	Token     string
	AccountID string // codex only
}

type Checker struct {
	CachePath string
	Client    *http.Client
	Now       func() time.Time
	ClaudeURL string
	CodexURL  string
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) readCache() map[string]Result {
	m := map[string]Result{}
	if b, ok, _ := fsutil.ReadFileIfExists(c.CachePath); ok {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// key names the login in the cache, like work:claude, never a token
func (c *Checker) Cached(key string) (*Result, bool) {
	r, ok := c.readCache()[key]
	if !ok || c.now().Sub(r.CheckedAt) >= TTL {
		return nil, false
	}
	return &r, true
}

func (c *Checker) Check(ctx context.Context, key string, l Login) (*Result, error) {
	if r, ok := c.Cached(key); ok {
		return r, nil
	}
	var r *Result
	var err error
	switch l.Adapter {
	case "claude":
		r, err = c.claude(ctx, l)
	case "codex":
		r, err = c.codex(ctx, l)
	default:
		return nil, fmt.Errorf("no usage check for %s", l.Adapter)
	}
	if err != nil {
		return nil, err
	}
	r.CheckedAt = c.now().UTC()
	m := c.readCache()
	m[key] = *r
	if b, err := json.Marshal(m); err == nil && c.CachePath != "" {
		_ = fsutil.WriteFileAtomic(c.CachePath, b, 0o600)
	}
	return r, nil
}

func (c *Checker) get(ctx context.Context, url string, headers map[string]string, into any) error {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cream-usage-check")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New("the login was refused, use the tool once as this profile so it refreshes its token")
	case resp.StatusCode != http.StatusOK:
		return errors.New("usage check: " + resp.Status)
	}
	return json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(into)
}

func (c *Checker) claude(ctx context.Context, l Login) (*Result, error) {
	type window struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    *string  `json:"resets_at"`
	}
	// the response carries many other fields of other shapes, only the known windows are read
	var body map[string]json.RawMessage
	url := c.ClaudeURL
	if url == "" {
		url = ClaudeURL
	}
	err := c.get(ctx, url, map[string]string{"Authorization": "Bearer " + l.Token, "anthropic-beta": "oauth-2025-04-20"}, &body)
	if err != nil {
		return nil, err
	}
	r := &Result{}
	for _, k := range []string{"five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet"} {
		var w *window
		if json.Unmarshal(body[k], &w) != nil || w == nil || w.Utilization == nil {
			continue
		}
		name := strings.NewReplacer("five_hour", "5h", "seven_day", "7d", "_", " ").Replace(k)
		win := Window{Name: name, Used: *w.Utilization}
		if w.ResetsAt != nil {
			win.ResetsAt, _ = time.Parse(time.RFC3339Nano, *w.ResetsAt)
		}
		r.Windows = append(r.Windows, win)
		r.Limited = r.Limited || win.Used >= 100
	}
	return r, nil
}

func (c *Checker) codex(ctx context.Context, l Login) (*Result, error) {
	type window struct {
		UsedPercent   float64 `json:"used_percent"`
		WindowSeconds int64   `json:"limit_window_seconds"`
		ResetAt       int64   `json:"reset_at"`
	}
	var body struct {
		RateLimit *struct {
			LimitReached bool    `json:"limit_reached"`
			Primary      *window `json:"primary_window"`
			Secondary    *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	headers := map[string]string{"Authorization": "Bearer " + l.Token}
	if l.AccountID != "" {
		headers["ChatGPT-Account-Id"] = l.AccountID
	}
	url := c.CodexURL
	if url == "" {
		url = CodexURL
	}
	if err := c.get(ctx, url, headers, &body); err != nil {
		return nil, err
	}
	r := &Result{}
	if body.RateLimit == nil {
		return r, nil
	}
	for _, w := range []*window{body.RateLimit.Primary, body.RateLimit.Secondary} {
		if w == nil {
			continue
		}
		win := Window{Name: windowName(w.WindowSeconds), Used: w.UsedPercent}
		if w.ResetAt > 0 {
			win.ResetsAt = time.Unix(w.ResetAt, 0).UTC()
		}
		r.Windows = append(r.Windows, win)
		r.Limited = r.Limited || win.Used >= 100
	}
	r.Limited = r.Limited || body.RateLimit.LimitReached
	return r, nil
}

// 18000 seconds is 5h, 604800 is 7d
func windowName(s int64) string {
	switch {
	case s <= 0:
		return "limit"
	case s%86400 == 0:
		return fmt.Sprintf("%dd", s/86400)
	case s%3600 == 0:
		return fmt.Sprintf("%dh", s/3600)
	}
	return fmt.Sprintf("%dm", (s+59)/60)
}
