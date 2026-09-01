package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
)

var videoPlayerCommand = func(ctx context.Context, name string, arguments ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arguments...)
}

func newVideoCommand(options *options) *cobra.Command {
	var playerPath string
	command := &cobra.Command{
		Use:     "video [selector]",
		Short:   "View a kart's live camera with ffplay",
		GroupID: groupDevice,
		Args:    cobra.MaximumNArgs(1),
		Example: `  komitake video
  komitake video XKW123
  komitake video --player /opt/homebrew/bin/ffplay`,
		RunE: func(command *cobra.Command, arguments []string) error {
			selector := ""
			if len(arguments) == 1 {
				selector = arguments[0]
			}
			return options.withClient(command, 0, func(ctx context.Context, client adminv1.AdminServiceClient) error {
				stream, err := client.StreamVideo(ctx, &adminv1.StreamVideoRequest{Selector: selector})
				if err != nil {
					return err
				}
				player := videoPlayerCommand(ctx, playerPath,
					"-hide_banner", "-loglevel", "warning", "-fflags", "nobuffer",
					"-flags", "low_delay", "-framedrop", "-f", "h264",
					"-framerate", "25", "-i", "pipe:0",
				)
				player.Stdout = command.OutOrStdout()
				player.Stderr = command.ErrOrStderr()
				stdin, err := player.StdinPipe()
				if err != nil {
					return fmt.Errorf("open ffplay input: %w", err)
				}
				if err := player.Start(); err != nil {
					return fmt.Errorf("start %s: %w", playerPath, err)
				}
				waitDone := make(chan error, 1)
				go func() { waitDone <- player.Wait() }()
				playerWaited := false
				defer func() {
					if playerWaited {
						return
					}
					_ = stdin.Close()
					if player.Process != nil {
						_ = player.Process.Kill()
					}
					<-waitDone
				}()
				for {
					frame, receiveErr := stream.Recv()
					if receiveErr != nil {
						if receiveErr == io.EOF {
							_ = stdin.Close()
							waitErr := <-waitDone
							playerWaited = true
							if waitErr != nil {
								return fmt.Errorf("ffplay exited: %w", waitErr)
							}
							return nil
						}
						if ctx.Err() != nil {
							return nil
						}
						return receiveErr
					}
					if frame.GetDiscontinuity() || len(frame.GetAnnexB()) == 0 {
						continue
					}
					payload := normalizeAnnexBForDecoder(frame.GetAnnexB())
					if _, err := stdin.Write(payload); err != nil {
						select {
						case waitErr := <-waitDone:
							playerWaited = true
							if waitErr != nil {
								return fmt.Errorf("ffplay exited: %w", waitErr)
							}
							return errors.New("ffplay exited before the video stream ended")
						default:
							return fmt.Errorf("write ffplay input: %w", err)
						}
					}
				}
			})
		},
	}
	command.Flags().StringVar(&playerPath, "player", "ffplay", "ffplay executable path")
	return command
}

func normalizeAnnexBForDecoder(payload []byte) []byte {
	trailingAUD := []byte{0, 0, 0, 1, 9, 0x30}
	if len(payload) >= len(trailingAUD) && string(payload[len(payload)-len(trailingAUD):]) == string(trailingAUD) {
		return payload[:len(payload)-len(trailingAUD)]
	}
	return payload
}
