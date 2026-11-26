// Package progress renders a single, self-overwriting status line on a
// terminal. On anything that is not a terminal (pipes, files, CI logs) every
// method is a no-op, so output stays clean.
package progress

import (
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// interval caps how often the line is redrawn.
const interval = 100 * time.Millisecond

// defaultWidth is used when the terminal width is unknown. Lines are cut to
// this width so they never wrap, which would break the redraw.
const defaultWidth = 80

// Line is a status line that redraws itself in place.
type Line struct {
	w       io.Writer
	enabled bool
	width   int
	ascii   bool
	now     func() time.Time
	last    time.Time
	frame   int
	shown   int // Width of the text currently on screen.
}

// New returns a Line writing to w. It is enabled only when w is a terminal.
func New(w io.Writer) *Line {
	return &Line{
		w:       w,
		enabled: isTerminal(w),
		width:   termWidth(),
		ascii:   runtime.GOOS == "windows", // Legacy consoles lack braille/block glyphs.
		now:     time.Now,
	}
}

// Enabled reports whether the line is drawn at all.
func (l *Line) Enabled() bool { return l.enabled }

// Tick redraws the line with render's text, at most once per interval. The
// render function receives the current spinner frame and is only called when
// the line is actually redrawn, so it may do work such as formatting paths.
func (l *Line) Tick(render func(spinner string) string) {
	if !l.enabled {
		return
	}
	now := l.now()
	if !l.last.IsZero() && now.Sub(l.last) < interval {
		return
	}
	l.last = now
	l.frame++
	l.draw(render(l.spinner()))
}

// Show redraws the line with text immediately, ignoring the interval. Use it
// for infrequent updates that each start slow work.
func (l *Line) Show(text string) {
	if !l.enabled {
		return
	}
	l.last = l.now()
	l.draw(text)
}

// Clear erases the line so that regular output can follow.
func (l *Line) Clear() {
	if !l.enabled || l.shown == 0 {
		return
	}
	l.write("\r" + strings.Repeat(" ", l.shown) + "\r")
	l.shown = 0
}

func (l *Line) draw(text string) {
	text = Truncate(text, l.width-1)
	n := utf8.RuneCountInString(text)
	pad := ""
	if n < l.shown {
		pad = strings.Repeat(" ", l.shown-n)
	}
	l.write("\r" + text + pad)
	l.shown = n
}

// write sends s to the terminal. Progress is best-effort: on a write error
// the line is disabled instead of failing the command.
func (l *Line) write(s string) {
	if _, err := io.WriteString(l.w, s); err != nil {
		l.enabled = false
	}
}

func (l *Line) spinner() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if l.ascii {
		frames = []string{"|", "/", "-", "\\"}
	}
	return frames[l.frame%len(frames)]
}

// Bar renders a width-cell progress bar for done out of total.
func (l *Line) Bar(done, total, width int) string {
	filled := 0
	if total > 0 {
		filled = min(width, done*width/total)
	}
	full, empty := "█", "░"
	if l.ascii {
		full, empty = "#", "-"
	}
	return "[" + strings.Repeat(full, filled) + strings.Repeat(empty, width-filled) + "]"
}

// Truncate shortens s to at most width runes, cutting the end and marking the
// cut with "…". Callers put the least important part (a path) last.
func Truncate(s string, width int) string {
	if width <= 0 || utf8.RuneCountInString(s) <= width {
		return s
	}
	r := []rune(s)
	return string(r[:width-1]) + "…"
}

// Count formats n with thousands separators, e.g. 48213 -> "48,213".
func Count(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + Count(-n)
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// termWidth reads $COLUMNS when the shell exports it and falls back to 80.
func termWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 10 {
		return n
	}
	return defaultWidth
}
