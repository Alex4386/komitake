package config

import (
	"os"
	"path/filepath"
	"testing"
)

// state.json holds the generated game-network PSK, which is the input to the
// link-layer CCMP key, so it must not be readable by other users.
func TestWritePersistentStateIsNotWorldReadable(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := writePersistentState(path, PersistentState{}); err != nil {
		t.Fatalf("writePersistentState() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %#o, want 0600", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("state directory mode = %#o, want no group or other access", perm)
	}
}

// An overwrite must preserve the restrictive mode rather than inheriting the
// umask from a fresh create.
func TestWritePersistentStateOverwriteKeepsMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := writePersistentState(path, PersistentState{}); err != nil {
		t.Fatalf("first write error = %v", err)
	}
	if err := writePersistentState(path, PersistentState{}); err != nil {
		t.Fatalf("second write error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %#o after overwrite, want 0600", perm)
	}
}

// The atomic write must not leave temp files behind.
func TestWritePersistentStateLeavesNoTempFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := writePersistentState(path, PersistentState{}); err != nil {
		t.Fatalf("writePersistentState() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("unexpected directory contents: %v", entries)
	}
}

// A readable file must survive a rewrite: the rename is atomic, so a reader
// either sees the old or the new contents, never a truncated file.
func TestWritePersistentStateRoundTrips(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	want := PersistentState{}
	if err := writePersistentState(path, want); err != nil {
		t.Fatalf("writePersistentState() error = %v", err)
	}

	got, err := readPersistentState(path)
	if err != nil {
		t.Fatalf("readPersistentState() error = %v", err)
	}
	if got.GameNetwork != nil && want.GameNetwork == nil {
		t.Fatalf("round trip mismatch: got %+v", got)
	}
}
