package tui

import (
	_ "embed"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

var (
	cBright = lipgloss.Color("#FAF6EC")
	cText   = lipgloss.Color("#E6E0D2")
	cAccent = lipgloss.Color("#4FB3A9")
	cHi     = lipgloss.Color("#F1E3C3")
	cGood   = lipgloss.Color("#7BD88F")
	cInk    = lipgloss.Color("#10201F")
	cBorder = lipgloss.Color("#2F3D3C")
	cDim    = lipgloss.Color("#8A9796")
	cFaint  = lipgloss.Color("#566463")
	cRed    = lipgloss.Color("#F2777A")
	cYellow = lipgloss.Color("#E8C468")
)

var gradientStops = []colorful.Color{
	hex("#4FB3A9"),
	hex("#9ED9CF"),
	hex("#F1E3C3"),
	hex("#FAF6EC"),
	hex("#4FB3A9"), // same as the first so the loop is seamless
}

func hex(s string) colorful.Color {
	c, _ := colorful.Hex(s)
	return c
}

func gradientAt(t float64) colorful.Color {
	t = t - math.Floor(t)
	seg := float64(len(gradientStops) - 1)
	pos := t * seg
	i := int(pos)
	if i >= len(gradientStops)-1 {
		i = len(gradientStops) - 2
	}
	return gradientStops[i].BlendLuv(gradientStops[i+1], pos-float64(i)).Clamped()
}

var (
	sBase     = lipgloss.NewStyle().Foreground(cText)
	sDim      = lipgloss.NewStyle().Foreground(cDim)
	sFaint    = lipgloss.NewStyle().Foreground(cFaint)
	sBold     = lipgloss.NewStyle().Foreground(cBright).Bold(true)
	sAccent   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sOK       = lipgloss.NewStyle().Foreground(cGood)
	sErr      = lipgloss.NewStyle().Foreground(cRed)
	sWarn     = lipgloss.NewStyle().Foreground(cYellow)
	sHi       = lipgloss.NewStyle().Foreground(cHi)
	sKey      = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sKeyDesc  = lipgloss.NewStyle().Foreground(cDim)
	sPanel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cBorder).Padding(0, 1)
	sPanelHot = sPanel.BorderForeground(cAccent)
	sModal    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cHi).Padding(1, 3)
	sBadgeDry = lipgloss.NewStyle().Foreground(cInk).Background(cYellow).Bold(true).Padding(0, 1)
	sBadgeUpd = lipgloss.NewStyle().Foreground(cInk).Background(cGood).Bold(true).Padding(0, 1)
	sTabOn    = lipgloss.NewStyle().Foreground(cInk).Background(cAccent).Bold(true).Padding(0, 1)
	sTabOff   = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
)

//go:embed logo.txt
var bigLogo string

var smallLogo = []string{
	"▄▀▀ █▀▄ █▀▀   ▄▀█   █▀▄▀█",
	"▀▄▄ █▀▄ ██▄ ▄ █▀█ ▄ █ ▀ █",
}

func bigLogoLines() []string {
	return strings.Split(strings.TrimRight(bigLogo, "\n"), "\n")
}

// reveal goes 0 to 1 and hides everything past the diagonal front for the intro
func paintGradient(lines []string, phase, reveal float64) string {
	width := 0
	for _, l := range lines {
		if n := len([]rune(l)); n > width {
			width = n
		}
	}
	if width == 0 {
		return ""
	}
	front := reveal * float64(width+len(lines)) * 1.15
	var b strings.Builder
	for y, l := range lines {
		runes := []rune(l)
		for x, r := range runes {
			if r == ' ' {
				b.WriteRune(' ')
				continue
			}
			d := float64(x + y)
			if reveal < 1 && d > front {
				b.WriteRune(' ')
				continue
			}
			c := gradientAt(phase + float64(x)/float64(width)*0.9 + float64(y)*0.015)
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
			if reveal < 1 && front-d < 3 {
				st = st.Foreground(cBright).Bold(true)
			}
			b.WriteString(st.Render(string(r)))
		}
		if y < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func shimmerText(s string, phase float64, base, hi lipgloss.Color) string {
	runes := []rune(s)
	n := float64(len(runes))
	center := math.Mod(phase*1.6, 1.6)*n - 0.3*n
	bc, _ := colorful.Hex(string(base))
	hc, _ := colorful.Hex(string(hi))
	var b strings.Builder
	for i, r := range runes {
		d := math.Abs(float64(i) - center)
		w := math.Max(0, 1-d/4)
		c := bc.BlendLuv(hc, w).Clamped()
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render(string(r)))
	}
	return b.String()
}
