package deviceselect

import (
	"strings"
	"testing"
)

func TestNormalizeSerial(t *testing.T) {
	t.Parallel()
	if got := NormalizeSerial("XKW-12.34"); got != "xkw1234" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveExactAndPrefix(t *testing.T) {
	t.Parallel()
	devs := []Device{
		{Ident: "aabbccddeeff00112233445566778899", Serial: "XKW123456789"},
		{Ident: "112233445566778899aabbccddeeff00", Serial: "XKW987654321"},
	}

	d, err := Resolve("XKW123456789", devs)
	if err != nil || d.Ident != devs[0].Ident {
		t.Fatalf("exact serial: %v %#v", err, d)
	}

	d, err = Resolve("xkw123", devs)
	if err != nil || d.Ident != devs[0].Ident {
		t.Fatalf("serial prefix: %v %#v", err, d)
	}

	d, err = Resolve(devs[1].Ident[:8], devs)
	if err != nil || d.Ident != devs[1].Ident {
		t.Fatalf("ident prefix: %v %#v", err, d)
	}
}

func TestResolveEmptySerialIgnored(t *testing.T) {
	t.Parallel()
	devs := []Device{
		{Ident: "aabbccddeeff00112233445566778899", Serial: ""},
	}
	_, err := Resolve("aabb", devs)
	if err != nil {
		t.Fatalf("ident should still match: %v", err)
	}
	_, err = Resolve("noserial", devs)
	if err == nil {
		t.Fatal("empty serial must not match arbitrary selectors")
	}
}

func TestResolveAmbiguous(t *testing.T) {
	t.Parallel()
	devs := []Device{
		{Ident: "aaaaaaaa", Serial: "1111"},
		{Ident: "aaaaabbb", Serial: "2222"},
	}
	_, err := Resolve("aaaa", devs)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("want ambiguous, got %v", err)
	}
}

func TestResolveExactWinsOverPrefix(t *testing.T) {
	t.Parallel()
	devs := []Device{
		{Ident: "aa", Serial: "xx"},
		{Ident: "aabb", Serial: "yy"},
	}
	d, err := Resolve("aa", devs)
	if err != nil || d.Ident != "aa" {
		t.Fatalf("exact should win: %v %#v", err, d)
	}
}

func TestResolveEmpty(t *testing.T) {
	t.Parallel()
	_, err := Resolve("  ", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
