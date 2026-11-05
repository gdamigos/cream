package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckCachesFor24h(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("update check must not send identifying headers")
		}
		w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://example/rel"}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	c := &Checker{CachePath: filepath.Join(t.TempDir(), "c.json"), Current: "v1.1.9", Endpoint: srv.URL, Now: func() time.Time { return now }}
	for i := 0; i < 3; i++ {
		r, err := c.Check(context.Background())
		if err != nil || !r.Newer || r.Latest != "v1.2.0" {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (cached)", hits.Load())
	}
	now = now.Add(25 * time.Hour)
	c.Check(context.Background())
	if hits.Load() != 2 {
		t.Fatal("cache did not expire after 24h")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		l, c string
		want bool
	}{
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.9", "1.0.0", false},
		{"v2.0.0", "dev", false},
		{"v1.10.0", "v1.9.3-rc1", true},
	}
	for _, c := range cases {
		if got := IsNewer(c.l, c.c); got != c.want {
			t.Errorf("IsNewer(%s,%s) = %v", c.l, c.c, got)
		}
	}
}

func TestDisabledInCI(t *testing.T) {
	t.Setenv("CREAM_NO_UPDATE_CHECK", "")
	t.Setenv("CI", "true")
	if !DisabledByEnv() {
		t.Fatal("should be disabled in CI")
	}
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CREAM_NO_UPDATE_CHECK", "1")
	if !DisabledByEnv() {
		t.Fatal("env opt-out ignored")
	}
}
