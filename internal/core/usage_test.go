package core

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/usage"
)

func loginCodex(t *testing.T, m *Manager, email, token, lastRefresh string) {
	t.Helper()
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	put(t, filepath.Join(m.Paths.Home, ".codex", "auth.json"),
		`{"tokens":{"id_token":"x.`+claims+`.y","access_token":"`+token+`","account_id":"acct-`+token+`"},"last_refresh":"`+lastRefresh+`"}`)
}

func codexProfiles(t *testing.T) *Manager {
	t.Helper()
	m, _ := newManager(t, []adapter.Adapter{adapters.Codex{}})
	for _, n := range []string{"work", "personal"} {
		if _, err := m.AddProfile(n, "", config.GitIdentity{}); err != nil {
			t.Fatal(err)
		}
	}
	loginCodex(t, m, "me@home.example.com", "tok-personal", "2026-01-01T00:00:00Z")
	if _, err := m.Import(ctx, "personal", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	loginCodex(t, m, "me@work.example.com", "tok-work", "2026-03-01T00:00:00Z")
	if _, err := m.Import(ctx, "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	m.Cfg.Active = "work"
	return m
}

func TestUsageAndSuggest(t *testing.T) {
	m := codexProfiles(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		used := map[string]string{"Bearer tok-work": "100", "Bearer tok-personal": "20"}[r.Header.Get("Authorization")]
		_, _ = w.Write([]byte(`{"rate_limit":{"limit_reached":` + map[bool]string{true: "true", false: "false"}[used == "100"] +
			`,"primary_window":{"used_percent":` + used + `,"limit_window_seconds":18000,"reset_at":1798761600}}}`))
	}))
	defer srv.Close()
	c := &usage.Checker{CodexURL: srv.URL, CachePath: m.Paths.UsageCache()}

	if us := m.Usage(ctx, c, false); us != nil {
		t.Fatal("usage ran with usage_check off")
	}
	m.Cfg.UsageCheck = true
	if us := m.Usage(ctx, c, true); len(us) != 2 || us[0].Result != nil {
		t.Fatalf("cached only should not ask, got %+v", us)
	}
	us := m.Usage(ctx, c, false)
	if len(us) != 2 || us[0].Profile != "personal" || us[0].Live || us[0].Result.Peak() != 20 || !us[1].Live || !us[1].Result.Limited {
		t.Fatalf("usage %+v", us)
	}
	s := Suggest(us)
	if len(s) != 1 || s[0].Adapter != "codex" || s[0].From != "work" || s[0].To != "personal" || !s[0].Until.Equal(time.Unix(1798761600, 0)) {
		t.Fatalf("suggestions %+v", s)
	}
	if strings.Contains(get(t, m.Paths.UsageCache()), "tok-") {
		t.Fatal("usage cache holds a token")
	}

	// a locked profile is neither asked nor suggested
	m.Cfg.Lock.Profiles = []string{"personal"}
	if s := Suggest(m.Usage(ctx, c, false)); len(s) != 0 {
		t.Fatalf("suggested a locked profile: %+v", s)
	}
}

func TestStaleLogins(t *testing.T) {
	m := codexProfiles(t)
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	// codex's own last_refresh counts, the active profile is skipped
	st := m.StaleLogins(now)
	if len(st) != 1 || st[0].Profile != "personal" || st[0].Days(now) != 73 {
		t.Fatalf("stale %+v", st)
	}
	if !strings.Contains(st[0].Fix(), "cream import personal --only codex") {
		t.Fatalf("fix %q", st[0].Fix())
	}
	days := 0
	m.Cfg.StaleDays = &days
	if len(m.StaleLogins(now)) != 0 {
		t.Fatal("stale_login_days = 0 should turn the warning off")
	}
	days = 100
	if len(m.StaleLogins(now)) != 0 {
		t.Fatal("73 days is not stale with stale_login_days = 100")
	}
}
