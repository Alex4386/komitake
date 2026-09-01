// Package deviceselect resolves adb-style device selectors against a list of
// connected karts. A selector matches RCD ident or device-reported serial
// (exact or unique prefix).
package deviceselect

import (
	"fmt"
	"strings"
)

// Device is the subset of kart identity needed for selection.
type Device struct {
	Ident      string
	Serial     string
	Kind       string
	Address    string
	MACAddress string
	SignalDBM  *int
}

// NormalizeSerial folds case and strips common separators for comparison.
func NormalizeSerial(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r == ':' || r == '-' || r == '.' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Resolve picks exactly one device for selector. Matching is case-insensitive
// and accepts:
//   - full ident or unique ident prefix
//   - full device-reported serial or unique serial prefix
//
// An empty selector is an error; callers that mean "any" should not use Resolve.
func Resolve(selector string, devices []Device) (Device, error) {
	sel := strings.ToLower(strings.TrimSpace(selector))
	if sel == "" {
		return Device{}, fmt.Errorf("empty device selector")
	}
	serialSel := NormalizeSerial(sel)

	var matches []Device
	for _, d := range devices {
		ident := strings.ToLower(d.Ident)
		serial := NormalizeSerial(d.Serial)
		identHit := ident == sel || strings.HasPrefix(ident, sel)
		serialHit := serial != "" && serialSel != "" &&
			(serial == serialSel || strings.HasPrefix(serial, serialSel))
		if identHit || serialHit {
			matches = append(matches, d)
		}
	}

	switch len(matches) {
	case 0:
		return Device{}, fmt.Errorf("no device matches %q", selector)
	case 1:
		return matches[0], nil
	default:
		// Prefer exact over prefix when both exist.
		var exact []Device
		for _, d := range matches {
			ident := strings.ToLower(d.Ident)
			serial := NormalizeSerial(d.Serial)
			if ident == sel || (serial != "" && serial == serialSel) {
				exact = append(exact, d)
			}
		}
		if len(exact) == 1 {
			return exact[0], nil
		}
		ids := make([]string, 0, len(matches))
		for _, d := range matches {
			ids = append(ids, d.Ident)
		}
		return Device{}, fmt.Errorf("ambiguous device selector %q; matches: %s", selector, strings.Join(ids, ", "))
	}
}
