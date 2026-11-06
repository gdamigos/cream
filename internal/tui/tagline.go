package tui

import (
	"math"
	"math/rand/v2"
	"strings"
)

// splash text
var taglines = []string{
	"one switch, every login",
	"log in once, switch forever",
	"no more logout, login, repeat",
	"work and personal, never mixed",
	"commits with the right email",
	"undo is one key away",
	"zero telemetry",
	"now with 100% more cream!",
	"not a dairy product",
	"lactose free!",
	"whipped, not stirred",
	"the cream always rises",
	"cream rules everything around me",
	"spilled milk? cream restore",
	"not affiliated with ice cream",
	"best served chilled",
	"works on my machine!",
	"git blame says it wasn't you",
	"it's not a bug, it's a profile",
	"alt-tab, but for logins",
	"also try cream run!",
	"your other account misses you",
	"one more profile won't hurt",
	"press ? for help, we won't judge",
	"tabs vs spaces?",
	"handcrafted in a terminal",
	"powered by coffee and Go",
	"10/10 would switch again",
	"less logging in, more shipping",
	"the cream of credential managers",
}

const (
	tagType  = 2.4 // seconds to type a phrase
	tagHold  = 6.0
	tagErase = 1.0
	tagGap   = 0.8
	tagCycle = tagType + tagHold + tagErase + tagGap
)

func taglineWidth() int {
	w := 0
	for _, s := range taglines {
		w = max(w, len([]rune(s)))
	}
	return w + 1
}

// every pass over the list gets its own shuffle from the session seed, so rendering stays a pure function of time
func taglineAt(seed, slot uint64) string {
	n := len(taglines)
	pass, i := slot/uint64(n), slot%uint64(n)
	order := rand.New(rand.NewPCG(seed, pass)).Perm(n)
	if pass > 0 {
		// never show the same phrase twice in a row across a reshuffle
		prev := rand.New(rand.NewPCG(seed, pass-1)).Perm(n)
		if order[0] == prev[n-1] {
			order[0], order[1] = order[1], order[0]
		}
	}
	return taglines[order[i]]
}

// types a phrase, holds it, erases it, then moves to the next one
func taglineText(seed uint64, t float64) (string, bool) {
	phrase := []rune(taglineAt(seed, uint64(max(0, t)/tagCycle)))
	p := math.Mod(t, tagCycle)
	n := len(phrase)
	typing := false
	switch {
	case p < tagType:
		n = int(float64(len(phrase)) * p / tagType)
		typing = true
	case p < tagType+tagHold:
	case p < tagType+tagHold+tagErase:
		n = int(float64(len(phrase)) * (1 - (p-tagType-tagHold)/tagErase))
	default:
		n = 0
	}
	return string(phrase[:n]), typing
}

// padded to a fixed width so the header never jumps around
func renderTagline(seed uint64, t float64) string {
	s, typing := taglineText(seed, t)
	out := ""
	if s != "" {
		out = paintGradient([]string{s}, t*0.05, 1)
	}
	if typing || math.Mod(t, 1) < 0.5 {
		out += sHi.Render("▌")
	} else {
		out += " "
	}
	return strings.Repeat(" ", taglineWidth()-len([]rune(s))-1) + out
}
