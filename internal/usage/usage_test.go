package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, body string, check func(*http.Request)) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		check(r)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s, &calls
}

func TestClaudeUsage(t *testing.T) {
	// trimmed from a real response, the extra fields of other shapes must not break parsing
	s, calls := server(t, `{"five_hour":{"utilization":100.0,"resets_at":"2026-11-09T15:00:00+00:00","limit_dollars":null},"seven_day":{"utilization":13.0,"resets_at":null},"seven_day_opus":null,
		"pace":null,"extra_usage":{"is_enabled":false,"utilization":null},"limits":[{"kind":"session","percent":100}],"spend":{"percent":0},"seven_day_breakdown":{"rows":[]}}`,
		func(r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok-claude" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
				t.Errorf("headers %v", r.Header)
			}
		})
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	cache := filepath.Join(t.TempDir(), "usage.json")
	c := &Checker{CachePath: cache, ClaudeURL: s.URL, Now: func() time.Time { return now }}
	r, err := c.Check(context.Background(), "work:claude", Login{Adapter: "claude", Token: "tok-claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Windows) != 2 || r.Windows[0].Name != "5h" || r.Windows[1].Name != "7d" || !r.Limited || r.Peak() != 100 {
		t.Fatalf("result %+v", r)
	}
	if want := time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC); !r.Until().Equal(want) {
		t.Fatalf("until %v", r.Until())
	}

	// cached for TTL, and the cache never holds the token
	if _, err := c.Check(context.Background(), "work:claude", Login{Adapter: "claude", Token: "tok-claude"}); err != nil || *calls != 1 {
		t.Fatalf("second check within the ttl hit the network, calls %d", *calls)
	}
	b, _ := os.ReadFile(cache)
	if strings.Contains(string(b), "tok-claude") {
		t.Fatal("the cache holds the token")
	}
	now = now.Add(TTL)
	if _, err := c.Check(context.Background(), "work:claude", Login{Adapter: "claude", Token: "tok-claude"}); err != nil || *calls != 2 {
		t.Fatalf("expired cache was not refreshed, calls %d", *calls)
	}
}

func TestCodexUsage(t *testing.T) {
	s, _ := server(t, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,
		"primary_window":{"used_percent":42,"limit_window_seconds":18000,"reset_after_seconds":60,"reset_at":1798761600},
		"secondary_window":{"used_percent":7,"limit_window_seconds":604800,"reset_after_seconds":60,"reset_at":1798761600}}}`,
		func(r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok-codex" || r.Header.Get("ChatGPT-Account-Id") != "acct-1" {
				t.Errorf("headers %v", r.Header)
			}
		})
	c := &Checker{CodexURL: s.URL}
	r, err := c.Check(context.Background(), "work:codex", Login{Adapter: "codex", Token: "tok-codex", AccountID: "acct-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Windows) != 2 || r.Windows[0].Name != "5h" || r.Windows[0].Used != 42 || r.Windows[1].Name != "7d" || r.Limited {
		t.Fatalf("result %+v", r)
	}
	if !r.Windows[0].ResetsAt.Equal(time.Unix(1798761600, 0)) {
		t.Fatalf("reset %v", r.Windows[0].ResetsAt)
	}
}

func TestRefusedLogin(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer s.Close()
	c := &Checker{ClaudeURL: s.URL}
	_, err := c.Check(context.Background(), "k", Login{Adapter: "claude", Token: "tok-secret"})
	if err == nil || strings.Contains(err.Error(), "tok-secret") {
		t.Fatalf("err = %v", err)
	}
}
