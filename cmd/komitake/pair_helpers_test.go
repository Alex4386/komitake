package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skip2/go-qrcode"
)

func testQR(t *testing.T) *qrcode.QRCode {
	t.Helper()
	qr, err := qrcode.New("komitake-test-payload", qrcode.Medium)
	if err != nil {
		t.Fatalf("qrcode.New() error = %v", err)
	}
	return qr
}

// The QR payload is equivalent to the pairing key, so the file must not be
// readable by other users.
func TestWriteQRFileUses0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qr.png")
	if err := writeQRFile(testQR(t), path); err != nil {
		t.Fatalf("writeQRFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %#o, want 0600", perm)
	}
	if info.Size() == 0 {
		t.Fatal("wrote an empty PNG")
	}
}

func TestWriteQRFileOverwritesExistingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qr.png")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := writeQRFile(testQR(t), path); err != nil {
		t.Fatalf("writeQRFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) == "existing" || len(data) < 8 {
		t.Fatalf("existing file was not replaced with a PNG")
	}
}

// rename replaces the symlink inode itself rather than writing through it, so a
// planted symlink cannot redirect the write into another file.
func TestWriteQRFileReplacesSymlinkWithoutTouchingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("sensitive"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	link := filepath.Join(dir, "qr.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := writeQRFile(testQR(t), link); err != nil {
		t.Fatalf("writeQRFile() error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "sensitive" {
		t.Fatalf("symlink target was overwritten: %q", data)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("path is still a symlink after write")
	}
}
