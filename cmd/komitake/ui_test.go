package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNewUIColorPrecedence(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		// A bytes.Buffer is never a TTY, so color is off unless forced.
		{name: "non-tty defaults off", env: nil, want: false},
		{name: "FORCE_COLOR=1 forces on", env: map[string]string{"FORCE_COLOR": "1"}, want: true},
		// FORCE_COLOR=0 must not enable color; checking presence alone is wrong.
		{name: "FORCE_COLOR=0 stays off", env: map[string]string{"FORCE_COLOR": "0"}, want: false},
		{name: "NO_COLOR beats FORCE_COLOR", env: map[string]string{"FORCE_COLOR": "1", "NO_COLOR": "1"}, want: false},
		{name: "empty NO_COLOR is ignored", env: map[string]string{"FORCE_COLOR": "1", "NO_COLOR": ""}, want: true},
		{name: "TERM=dumb disables", env: map[string]string{"FORCE_COLOR": "1", "TERM": "dumb"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv registers restoration of the original value; unsetting
			// afterwards gives a clean slate that is still rolled back on exit.
			for _, key := range []string{"NO_COLOR", "FORCE_COLOR", "TERM"} {
				t.Setenv(key, "")
				if err := os.Unsetenv(key); err != nil {
					t.Fatalf("Unsetenv(%q) error = %v", key, err)
				}
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var out bytes.Buffer
			if got := newUI(&out, &out).color; got != tt.want {
				t.Fatalf("color = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldAlignmentIgnoresANSIWidth(t *testing.T) {
	var out bytes.Buffer
	u := newUI(&out, &out)
	u.color = true
	u.c = styled

	u.Field(10, "ssid", "value")

	got := out.String()
	// Styling must wrap the already-padded label, otherwise the width verb
	// counts escape bytes and columns drift.
	if !strings.Contains(got, styled.dim+"ssid:      "+styled.reset) {
		t.Fatalf("padding applied outside styling: %q", got)
	}
}

func TestTableRendersTabsWhenNotTTY(t *testing.T) {
	var out bytes.Buffer
	u := newUI(&out, &out)

	tbl := newTable("serial", "kind")
	tbl.Add("SERIAL1", "Fuji")
	tbl.Render(u)

	got := out.String()
	if got != "SERIAL1\tFuji\n" {
		t.Fatalf("non-tty output = %q, want tab-separated with no header", got)
	}
}

func TestTableAlignsAndPadsForTTY(t *testing.T) {
	var out bytes.Buffer
	u := newUI(&out, &out)
	u.tty = true

	tbl := newTable("serial", "kind")
	tbl.Add("LONG-SERIAL-1", "Fuji")
	tbl.Add("S2", "Fuji")
	tbl.Render(u)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header plus 2 rows, got %d: %q", len(lines), out.String())
	}
	if !strings.HasPrefix(lines[0], "SERIAL") {
		t.Fatalf("missing header row: %q", lines[0])
	}
	// The short serial is padded so the second column starts at one offset.
	if strings.Index(lines[1], "Fuji") != strings.Index(lines[2], "Fuji") {
		t.Fatalf("columns misaligned:\n%q\n%q", lines[1], lines[2])
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("trailing whitespace in %q", line)
		}
	}
}

// Add pads short rows so a missing trailing cell cannot panic the renderer.
func TestTableAddPadsMissingCells(t *testing.T) {
	var out bytes.Buffer
	u := newUI(&out, &out)

	tbl := newTable("a", "b", "c")
	tbl.Add("1")
	tbl.Render(u)

	if got := out.String(); got != "1\t-\t-\n" {
		t.Fatalf("output = %q", got)
	}
}
