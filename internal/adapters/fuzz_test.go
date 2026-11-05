package adapters

import (
	"strings"
	"testing"
)

// the user's gitconfig is arbitrary text, replacing our block must keep it and be repeatable
func FuzzReplaceBlock(f *testing.F) {
	block := BlockBegin + "\n[user]\n\tuseConfigOnly = true\n" + BlockEnd + "\n"
	f.Add("[core]\n\tautocrlf = false\n")
	f.Add("")
	f.Add(block)
	f.Add("[user]\n\temail = a@b\n" + block + "[alias]\n\tst = status\n")
	f.Add(BlockBegin + "\nunterminated")
	f.Fuzz(func(t *testing.T, user string) {
		once := ReplaceBlock(user, block)
		if strings.Count(once, BlockBegin) != 1 || !strings.HasSuffix(once, block) {
			t.Fatalf("block not placed once at the end:\n%q", once)
		}
		if twice := ReplaceBlock(once, block); twice != once {
			t.Fatalf("not idempotent:\n%q\n%q", once, twice)
		}
		if !strings.Contains(user, BlockBegin) && !strings.HasPrefix(once, strings.TrimRight(user, "\n")) {
			t.Fatalf("user config lost:\n%q", once)
		}
		_ = HasGlobalEmailOutsideBlock(once)
	})
}
