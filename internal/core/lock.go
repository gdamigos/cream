package core

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

func (m *Manager) checkUnlocked(profile string) error {
	if m.Cfg.Lock.All {
		return errors.New("cream is locked (run `cream unlock` first)")
	}
	if m.Cfg.Locked(profile) {
		return fmt.Errorf("profile %q is locked (run `cream unlock %s` first)", profile, profile)
	}
	return nil
}

func lockScope(profile string) string {
	if profile == "" {
		return vault.LockAll
	}
	return profile
}

func lockLabel(profile string) string {
	if profile == "" {
		return "everything"
	}
	return profile
}

// logs the tools out on purpose and turns git identity off, an empty profile means everything
func (m *Manager) Lock(ctx context.Context, profile string, dryRun bool, progress Progress) ([]Event, error) {
	if m.Cfg.Lock.All {
		return nil, errors.New("everything is already locked (run `cream unlock`)")
	}
	var p *config.Profile
	if profile != "" {
		var err error
		if p, err = m.Cfg.Profile(profile); err != nil {
			return nil, err
		}
		if m.Cfg.Locked(profile) {
			return nil, fmt.Errorf("profile %q is already locked", profile)
		}
	}
	scope := lockScope(profile)

	// refreshed tokens go into the profile first, so unlocking or switching back is never stale
	if !dryRun {
		owner := p
		if owner == nil {
			owner = m.Cfg.Profiles[m.Cfg.Active]
		}
		if owner != nil {
			m.syncOut(ctx, owner, m.Adapters)
		}
	}

	snap, err := m.capture(m.Adapters, "lock-"+lockLabel(profile), m.Cfg.Active, m.Cfg.Active)
	if err != nil {
		return nil, err
	}
	if !dryRun {
		if _, err := m.Backups.Save(snap); err != nil {
			return nil, fmt.Errorf("backup failed, nothing was changed: %w", err)
		}
	}

	var events []Event
	var done []adapter.Adapter
	undo := func(cause error) error {
		var errs []error
		for _, a := range done {
			if _, _, err := a.(adapter.Locker).Unlock(ctx, m.Env, vault.LockKey(scope, a.ID()), false); err != nil {
				errs = append(errs, err)
			}
		}
		errs = append(errs, m.rollback(snap, m.Adapters))
		if rerr := errors.Join(errs...); rerr != nil {
			return fmt.Errorf("%w (undo also failed: %w; restore with `cream restore`)", cause, rerr)
		}
		return fmt.Errorf("%w (nothing was locked)", cause)
	}

	for _, a := range m.Adapters {
		l, ok := a.(adapter.Locker)
		if !ok {
			continue
		}
		account := ""
		if p != nil {
			acct := p.Accounts[a.ID()]
			if acct == nil {
				ev := Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "no saved account"}
				events = append(events, ev)
				emit(progress, ev)
				continue
			}
			account = acct.Label
		}
		emit(progress, Event{Adapter: a.ID(), Name: a.Name(), State: Running})
		locked, ch, err := l.Lock(ctx, m.Env, vault.LockKey(scope, a.ID()), account, dryRun)
		var ev Event
		switch {
		case err != nil:
			ev = Event{Adapter: a.ID(), Name: a.Name(), State: Failed, Msg: err.Error()}
		case !locked && account != "":
			ev = Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "not logged in as " + account}
		case !locked:
			ev = Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "not logged in"}
		default:
			ev = Event{Adapter: a.ID(), Name: a.Name(), State: Done, Msg: would(dryRun, "logged out", "would log out"), Changes: ch}
		}
		events = append(events, ev)
		emit(progress, ev)
		if err != nil && !dryRun {
			if locked {
				done = append(done, a)
			}
			return events, undo(fmt.Errorf("%s: %w", a.ID(), err))
		}
		if locked {
			done = append(done, a)
		}
	}

	prev := m.Cfg.Lock
	prev.Profiles = slices.Clone(prev.Profiles)
	if p == nil {
		m.Cfg.Lock.All = true
	} else {
		m.Cfg.Lock.Profiles = append(m.Cfg.Lock.Profiles, profile)
	}
	ch, err := m.SyncGit(dryRun)
	events = append(events, gitEvent(ch, err, would(dryRun, "identity off", "would turn identity off")))
	if err != nil && !dryRun {
		m.Cfg.Lock = prev
		return events, undo(fmt.Errorf("git: %w", err))
	}
	if dryRun {
		m.Cfg.Lock = prev
		return events, nil
	}
	if err := m.Save(); err != nil {
		return events, err
	}
	m.Log("lock %s", lockLabel(profile))
	return events, nil
}

func would(dryRun bool, done, dry string) string {
	if dryRun {
		return dry
	}
	return done
}

func gitEvent(ch []adapter.Change, err error, msg string) Event {
	ev := Event{Adapter: "git", Name: "Git identity", State: Done, Msg: msg, Changes: ch}
	switch {
	case err != nil:
		ev.State, ev.Msg = Failed, err.Error()
	case len(ch) == 0:
		ev.State, ev.Msg = Skipped, "nothing to change"
	}
	return ev
}

// puts the saved logins back, an empty profile unlocks everything including locked profiles
// a failure stops here and keeps the lock, running unlock again retries what is left
func (m *Manager) Unlock(ctx context.Context, profile string, dryRun bool, progress Progress) ([]Event, error) {
	var scopes []string
	if profile == "" {
		if !m.Cfg.AnyLocked() {
			return nil, errors.New("nothing is locked")
		}
		// profiles first, so the logins saved by the everything lock end up live
		scopes = append(slices.Clone(m.Cfg.Lock.Profiles), vault.LockAll)
	} else {
		if _, err := m.Cfg.Profile(profile); err != nil {
			return nil, err
		}
		if !slices.Contains(m.Cfg.Lock.Profiles, profile) {
			if m.Cfg.Lock.All {
				return nil, errors.New("everything is locked (run `cream unlock` without a profile)")
			}
			return nil, fmt.Errorf("profile %q is not locked", profile)
		}
		scopes = []string{profile}
	}

	snap, err := m.capture(m.Adapters, "unlock-"+lockLabel(profile), m.Cfg.Active, m.Cfg.Active)
	if err != nil {
		return nil, err
	}
	if !dryRun {
		if _, err := m.Backups.Save(snap); err != nil {
			return nil, fmt.Errorf("backup failed, nothing was changed: %w", err)
		}
	}

	var events []Event
	restoredGitHub := false
	for _, scope := range scopes {
		for _, a := range m.Adapters {
			l, ok := a.(adapter.Locker)
			if !ok {
				continue
			}
			key := vault.LockKey(scope, a.ID())
			// a profile unlock never overwrites a login made since, the profile already holds its tokens
			// gh keeps several accounts, so its login is always added back
			if scope != vault.LockAll && a.ID() != "github" {
				if st := a.Status(ctx, m.Env); st.LoggedIn {
					if _, err := m.Env.Vault.Get(key); err == nil {
						if !dryRun {
							_ = m.Env.Vault.Delete(key)
						}
						ev := Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "kept the current login (" + st.Account + ")"}
						events = append(events, ev)
						emit(progress, ev)
					}
					continue
				}
			}
			restored, ch, err := l.Unlock(ctx, m.Env, key, dryRun)
			if !restored && err == nil {
				continue
			}
			ev := Event{Adapter: a.ID(), Name: a.Name(), State: Done, Msg: would(dryRun, "logged back in", "would log back in"), Changes: ch}
			if err != nil {
				ev.State, ev.Msg = Failed, err.Error()
			}
			events = append(events, ev)
			emit(progress, ev)
			if err != nil && !dryRun {
				return events, fmt.Errorf("%s: %w (still locked, run unlock again to retry)", a.ID(), err)
			}
			if a.ID() == "github" {
				restoredGitHub = true
			}
		}
	}

	prev := m.Cfg.Lock
	prev.Profiles = slices.Clone(prev.Profiles)
	if profile == "" {
		m.Cfg.Lock = config.LockState{}
	} else {
		m.Cfg.Lock.Profiles = slices.DeleteFunc(m.Cfg.Lock.Profiles, func(p string) bool { return p == profile })
	}
	ch, err := m.SyncGit(dryRun)
	events = append(events, gitEvent(ch, err, would(dryRun, "identity back", "would turn identity back on")))
	if dryRun {
		m.Cfg.Lock = prev
		return events, nil
	}
	if err != nil {
		m.Cfg.Lock = prev
		return events, fmt.Errorf("git: %w (still locked, run unlock again to retry)", err)
	}
	// gh may have moved to another account meanwhile, make the active profile's account current again
	if restoredGitHub && profile != "" && m.Cfg.Active == profile {
		for _, a := range m.Adapters {
			if a.ID() == "github" {
				_, _ = a.Activate(ctx, m.Env, m.Cfg.Profiles[profile], false)
			}
		}
	}
	if err := m.Save(); err != nil {
		return events, err
	}
	m.Log("unlock %s", lockLabel(profile))
	return events, nil
}
