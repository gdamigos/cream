package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTaglineCycle(t *testing.T) {
	const seed = 42
	if s, typing := taglineText(seed, 0); s != "" || !typing {
		t.Fatalf("start: got %q typing=%v", s, typing)
	}
	if s, _ := taglineText(seed, tagType+1); s != taglineAt(seed, 0) {
		t.Fatalf("hold: got %q", s)
	}
	if s, _ := taglineText(seed, tagCycle-tagGap/2); s != "" {
		t.Fatalf("gap: got %q", s)
	}
	if s, _ := taglineText(seed, tagCycle+tagType+1); s != taglineAt(seed, 1) {
		t.Fatalf("next phrase: got %q", s)
	}
}

func TestTaglineShuffle(t *testing.T) {
	n := uint64(len(taglines))
	for seed := uint64(0); seed < 20; seed++ {
		prev := ""
		for pass := uint64(0); pass < 5; pass++ {
			seen := map[string]bool{}
			for i := uint64(0); i < n; i++ {
				s := taglineAt(seed, pass*n+i)
				if s == prev {
					t.Fatalf("seed %d: %q shown twice in a row", seed, s)
				}
				seen[s], prev = true, s
			}
			if uint64(len(seen)) != n {
				t.Fatalf("seed %d pass %d: only %d of %d phrases shown", seed, pass, len(seen), n)
			}
		}
	}
	same := true
	for i := uint64(0); i < n; i++ {
		if taglineAt(1, i) != taglineAt(2, i) {
			same = false
		}
	}
	if same {
		t.Fatal("different seeds gave the same order")
	}
}

func TestTaglineFixedWidth(t *testing.T) {
	w := taglineWidth()
	for f := 0.0; f < tagCycle*3; f += 0.05 {
		if got := ansi.StringWidth(renderTagline(7, f)); got != w {
			t.Fatalf("t=%.2f width %d, want %d", f, got, w)
		}
	}
}
