// the update check, keep it notify only and free of identifiers, the only other network call is the opt-in internal/usage
package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamigos/cream/internal/fsutil"
)

const Repo = "gdamigos/cream"

const Interval = 24 * time.Hour

type Result struct {
	Latest    string    `json:"latest"`
	URL       string    `json:"url"`
	CheckedAt time.Time `json:"checked_at"`
	Newer     bool      `json:"-"`
}

func DisabledByEnv() bool {
	if v := os.Getenv("CREAM_NO_UPDATE_CHECK"); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return true
	}
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "TF_BUILD", "JENKINS_URL"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

type Checker struct {
	CachePath string
	Current   string
	Endpoint  string
	Client    *http.Client
	Now       func() time.Time
}

func (c *Checker) Check(ctx context.Context) (*Result, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	if b, ok, _ := fsutil.ReadFileIfExists(c.CachePath); ok {
		var r Result
		if json.Unmarshal(b, &r) == nil && now().Sub(r.CheckedAt) < Interval {
			r.Newer = IsNewer(r.Latest, c.Current)
			return &r, nil
		}
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://api.github.com/repos/" + Repo + "/releases/latest"
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cream-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r := Result{CheckedAt: now().UTC()}
	if resp.StatusCode == http.StatusOK {
		var body struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
			return nil, err
		}
		r.Latest, r.URL = body.TagName, body.HTMLURL
	} else if resp.StatusCode != http.StatusNotFound {
		return nil, errors.New("update check: " + resp.Status)
	}
	if b, err := json.Marshal(r); err == nil {
		_ = fsutil.WriteFileAtomic(c.CachePath, b, 0o600)
	}
	r.Newer = IsNewer(r.Latest, c.Current)
	return &r, nil
}

func IsNewer(latest, current string) bool {
	l, okL := parse(latest)
	c, okC := parse(current)
	if !okL || !okC {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return out, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
