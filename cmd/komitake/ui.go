package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/mattn/go-isatty"
)

// ANSI escape sequences. Empty strings when styling is disabled, so call sites
// can interpolate unconditionally.
type palette struct {
	reset, bold, dim         string
	red, green, yellow, cyan string
}

var styled = palette{
	reset:  "\033[0m",
	bold:   "\033[1m",
	dim:    "\033[2m",
	red:    "\033[31m",
	green:  "\033[32m",
	yellow: "\033[33m",
	cyan:   "\033[36m",
}

var plain = palette{}

// ui renders command output, adapting to whether the destination is a terminal.
type ui struct {
	out   io.Writer
	err   io.Writer
	color bool
	tty   bool
	c     palette
}

// isTerminal reports whether w is an interactive terminal. Buffers used by
// tests and pipes both return false, which keeps output byte-stable. Uses
// go-isatty so it also recognizes terminals that a raw ModeCharDevice check
// misses (for example mintty/cygwin on Windows).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// truthyEnv reports whether an environment variable is set to something other
// than an explicit off value. FORCE_COLOR=0 means "do not force", so presence
// alone is not enough.
func truthyEnv(name string) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// newUI decides on styling from the destination and the conventional
// environment overrides. Precedence: NO_COLOR beats FORCE_COLOR, which beats
// TTY detection.
func newUI(out, errOut io.Writer) *ui {
	tty := isTerminal(out)

	color := tty
	if truthyEnv("FORCE_COLOR") {
		color = true
	}
	if os.Getenv("TERM") == "dumb" {
		color = false
	}
	// NO_COLOR is honored last so it always wins; see https://no-color.org.
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		color = false
	}

	u := &ui{out: out, err: errOut, color: color, tty: tty, c: plain}
	if color {
		u.c = styled
	}
	return u
}

func (u *ui) paint(style, text string) string {
	if !u.color || style == "" {
		return text
	}
	return style + text + u.c.reset
}

func (u *ui) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(u.out, format, args...)
}

func (u *ui) Println(args ...any) {
	_, _ = fmt.Fprintln(u.out, args...)
}

// Warnf writes a non-fatal notice to stderr so it never pollutes piped stdout.
func (u *ui) Warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(u.err, "%s %s\n",
		u.paint(u.c.yellow, "warning:"),
		strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
}

func (u *ui) Success(msg string) {
	u.Printf("%s %s\n", u.paint(u.c.green, "OK"), msg)
}

// Field prints an aligned "label: value" line. width is the label column size.
// Padding is applied before styling; ANSI bytes would otherwise be counted by
// the width verb and break alignment.
func (u *ui) Field(width int, label, format string, args ...any) {
	padded := fmt.Sprintf("%-*s", width+1, label+":")
	u.Printf("  %s %s\n", u.paint(u.c.dim, padded), fmt.Sprintf(format, args...))
}

func (u *ui) Heading(text string) {
	u.Printf("%s\n", u.paint(u.c.bold, text))
}

// clearScreen resets the cursor for repainting in watch mode. It is a no-op on
// non-terminals so redirected output stays append-only.
func (u *ui) clearScreen() {
	if !u.tty || !u.color {
		return
	}
	u.Printf("\033[H\033[2J")
}

// spinner wraps briandowns/spinner with the terminal-awareness the rest of the
// UI follows: on a non-TTY it degrades to a single static line so piped or
// redirected output stays append-only and byte-stable.
type uiSpinner struct {
	u   *ui
	s   *spinner.Spinner
	msg string
}

// Spinner starts an animated progress indicator with the given message. Call
// Stop (or StopSuccess/StopFail) when the work resolves. On a non-TTY it prints
// the message once and animates nothing.
func (u *ui) Spinner(msg string) *uiSpinner {
	sp := &uiSpinner{u: u, msg: msg}
	if !u.tty {
		fmt.Fprintf(u.out, "%s\n", msg)
		return sp
	}
	// CharSets[14] is the braille dot cycle: compact and widely legible.
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(u.out))
	_ = s.Color("cyan")
	s.Suffix = " " + msg
	s.Start()
	sp.s = s
	return sp
}

// Update changes the spinner suffix mid-flight.
func (sp *uiSpinner) Update(msg string) {
	sp.msg = msg
	if sp.s != nil {
		sp.s.Suffix = " " + msg
	}
}

// Stop halts the spinner and clears its line, leaving nothing behind.
func (sp *uiSpinner) Stop() {
	if sp.s != nil {
		sp.s.Stop()
		sp.u.clearLine()
	}
}

// StopSuccess halts the spinner and prints a success line in its place.
func (sp *uiSpinner) StopSuccess(msg string) {
	sp.Stop()
	sp.u.Success(msg)
}

// StopFail halts the spinner without printing; the caller reports the error.
func (sp *uiSpinner) StopFail() {
	sp.Stop()
}

// clearLine erases in-place progress output, such as a spinner, before printing
// a final result over it.
func (u *ui) clearLine() {
	if !u.tty {
		return
	}
	u.Printf("\r\033[K")
}
