package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// testLine returns an enabled Line writing to buf with a controllable clock.
func testLine(buf *bytes.Buffer, clock *time.Time) *Line {
	return &Line{w: buf, enabled: true, width: 40, ascii: true, now: func() time.Time { return *clock }}
}

func TestNewDisabledForNonTerminal(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	if l.Enabled() {
		t.Fatal("enabled for a bytes.Buffer")
	}
	l.Tick(func(string) string { return "x" })
	l.Show("x")
	l.Clear()
	if buf.Len() != 0 {
		t.Errorf("wrote %q to a non-terminal", buf.String())
	}
}

func TestTickThrottles(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(0, 0)
	l := testLine(&buf, &clock)
	calls := 0
	render := func(string) string { calls++; return "scan" }

	l.Tick(render) // first tick always draws
	clock = clock.Add(interval / 2)
	l.Tick(render) // too soon
	clock = clock.Add(interval)
	l.Tick(render)

	if calls != 2 {
		t.Errorf("render called %d times, want 2", calls)
	}
}

func TestDrawPadsAndClear(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(0, 0)
	l := testLine(&buf, &clock)

	l.Show("longer text")
	l.Show("short")
	if got, want := buf.String(), "\rlonger text\rshort      "; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}

	buf.Reset()
	l.Clear()
	if got, want := buf.String(), "\r     \r"; got != want {
		t.Errorf("Clear() = %q, want %q", got, want)
	}
	buf.Reset()
	l.Clear() // nothing on screen
	if buf.Len() != 0 {
		t.Errorf("second Clear() wrote %q", buf.String())
	}
}

func TestDrawTruncatesToWidth(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(0, 0)
	l := testLine(&buf, &clock)
	l.Show(strings.Repeat("x", 100))
	if got := len([]rune(strings.TrimPrefix(buf.String(), "\r"))); got != l.width-1 {
		t.Errorf("drawn width = %d, want %d", got, l.width-1)
	}
}

func TestBar(t *testing.T) {
	l := &Line{ascii: true}
	tests := []struct {
		done, total int
		want        string
	}{
		{0, 10, "[----------]"},
		{5, 10, "[#####-----]"},
		{10, 10, "[##########]"},
		{0, 0, "[----------]"},
	}
	for _, tt := range tests {
		if got := l.Bar(tt.done, tt.total, 10); got != tt.want {
			t.Errorf("Bar(%d, %d) = %q, want %q", tt.done, tt.total, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("abcdef", 4); got != "abc…" {
		t.Errorf("Truncate() = %q", got)
	}
	if got := Truncate("abc", 4); got != "abc" {
		t.Errorf("Truncate() = %q", got)
	}
	if got := Truncate("⠋ ~/git/αβγ", 5); got != "⠋ ~/…" {
		t.Errorf("Truncate() multibyte = %q", got)
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 48213: "48,213", 1234567: "1,234,567", -1000: "-1,000"} {
		if got := Count(n); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}
