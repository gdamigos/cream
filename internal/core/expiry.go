package core

import (
	"fmt"
	"time"
)

type StaleLogin struct {
	Profile, Adapter, Name, Label string
	Since                         time.Time // when the login was last refreshed
}

func (s StaleLogin) Days(now time.Time) int { return int(now.Sub(s.Since).Hours() / 24) }

func (s StaleLogin) Msg(now time.Time) string {
	return fmt.Sprintf("%s's %s login (%s) was last refreshed %d days ago and may have expired", s.Profile, s.Name, s.Label, s.Days(now))
}

func (s StaleLogin) Fix() string {
	return "cream use " + s.Profile + " --only " + s.Adapter + " and run it once, or log in again and cream import " + s.Profile + " --only " + s.Adapter
}

// saved logins of the other profiles that haven't been refreshed for stale_login_days
// reads config only, the active profile is skipped because its tools refresh their own logins
func (m *Manager) StaleLogins(now time.Time) []StaleLogin {
	days := m.Cfg.StaleLoginDays()
	if days == 0 {
		return nil
	}
	var out []StaleLogin
	for _, name := range m.Cfg.Names() {
		p := m.Cfg.Profiles[name]
		if name == m.Cfg.Active || p.Protected() {
			continue
		}
		for _, a := range m.Adapters {
			acct := p.Accounts[a.ID()]
			if acct == nil || !secretAdapters[a.ID()] {
				continue
			}
			since := acct.LastRefresh()
			if !since.IsZero() && now.Sub(since) >= time.Duration(days)*24*time.Hour {
				out = append(out, StaleLogin{Profile: name, Adapter: a.ID(), Name: a.Name(), Label: acct.Label, Since: since})
			}
		}
	}
	return out
}
