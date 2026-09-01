package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (m *Manager) persistPairingLocked(record *PairingRecord) error {
	path := m.cfg.PairingFile
	if path == "" {
		return nil
	}

	payload := PairingRecord{State: m.state, FilePath: path}
	if record != nil {
		payload = *record
	}

	// 0700 / 0600: the record carries the pairing seed, and the pairing PSK is
	// SHA-256(seed), so a readable file hands over the network key.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	m.logger.Debug("pairing state persisted", "path", path, "state", payload.State)
	return nil
}

// writeFileAtomic writes via a temp file and rename so a crash mid-write cannot
// leave a truncated file that fails to parse on the next start.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	// CreateTemp makes the file 0600; set the requested mode explicitly so the
	// result does not depend on that default.
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
