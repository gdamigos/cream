package core

import (
	"context"
	"errors"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/usage"
)

type Usage struct {
	Profile, Adapter, Name string
	Live                   bool // the tool is logged in as this profile right now
	Result                 *usage.Result
	Err                    string
}

func (m *Manager) usageChecker() *usage.Checker {
	return &usage.Checker{CachePath: m.Paths.UsageCache()}
}

// plan usage of every saved Claude Code and Codex login, only when usage_check is on
// cached reads only when cachedOnly, so a caller can draw something before any request
func (m *Manager) Usage(ctx context.Context, c *usage.Checker, cachedOnly bool) []Usage {
	if !m.Cfg.UsageCheck || m.Cfg.Lock.All {
		return nil
	}
	if c == nil {
		c = m.usageChecker()
	}
	live := map[string]string{}
	for _, a := range m.Adapters {
		if _, ok := a.(adapter.UsageReporter); ok {
			if st := a.Status(ctx, m.Env); st.LoggedIn {
				live[a.ID()] = st.Account
			}
		}
	}
	var out []Usage
	for _, name := range m.Cfg.Names() {
		p := m.Cfg.Profiles[name]
		if m.Cfg.Locked(name) {
			continue
		}
		for _, a := range m.Adapters {
			ur, ok := a.(adapter.UsageReporter)
			acct := p.Accounts[a.ID()]
			if !ok || acct == nil {
				continue
			}
			u := Usage{Profile: name, Adapter: a.ID(), Name: a.Name(), Live: live[a.ID()] == acct.Label}
			key := name + ":" + a.ID()
			if r, ok := c.Cached(key); ok {
				u.Result = r
			} else if !cachedOnly {
				token, account, err := ur.UsageLogin(ctx, m.Env, p, u.Live)
				if err == nil {
					u.Result, err = c.Check(ctx, key, usage.Login{Adapter: a.ID(), Token: token, AccountID: account})
				}
				var skip adapter.SkipError
				switch {
				case errors.As(err, &skip):
					u.Err = skip.Reason
				case err != nil:
					u.Err = err.Error()
				}
			}
			out = append(out, u)
		}
	}
	return out
}

// a tool whose live login hit its limit and a profile that still has room
type Suggestion struct {
	Adapter, Name, From, To string
	Until                   time.Time
}

func Suggest(us []Usage) []Suggestion {
	var out []Suggestion
	for _, u := range us {
		if !u.Live || u.Result == nil || !u.Result.Limited {
			continue
		}
		var best *Usage
		for i, o := range us {
			if o.Adapter != u.Adapter || o.Profile == u.Profile || o.Result == nil || o.Result.Limited {
				continue
			}
			if best == nil || o.Result.Peak() < best.Result.Peak() {
				best = &us[i]
			}
		}
		if best != nil {
			out = append(out, Suggestion{Adapter: u.Adapter, Name: u.Name, From: u.Profile, To: best.Profile, Until: u.Result.Until()})
		}
	}
	return out
}
