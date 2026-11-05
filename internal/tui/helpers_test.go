package tui

import (
	"context"
	"fmt"
)

func fmtType(v any) string { return fmt.Sprintf("%T", v) }

func ctxBG() context.Context { return context.Background() }
