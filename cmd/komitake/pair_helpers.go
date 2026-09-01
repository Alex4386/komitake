package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/skip2/go-qrcode"
)

// bestEffortRestoreRunning leaves pairing mode when the command exits. It uses a
// fresh context so it still runs after the caller's context is canceled.
func bestEffortRestoreRunning(client adminv1.AdminServiceClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	state, err := client.GetState(ctx, &adminv1.GetStateRequest{})
	if err != nil || state.State != adminv1.State_STATE_PAIRING {
		return
	}
	_, _ = client.SetState(ctx, &adminv1.SetStateRequest{State: adminv1.State_STATE_RUNNING})
}

// writeQRFile writes the PNG at 0600. Uses a temp file + rename so an existing
// path can be overwritten without following a symlink (O_EXCL alone made every
// re-pair require a manual rm).
func writeQRFile(qr *qrcode.QRCode, path string) error {
	png, err := qr.PNG(256)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".komitake-qr-*.png")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(png); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func removeQRFile(path string) {
	_ = os.Remove(path)
}
