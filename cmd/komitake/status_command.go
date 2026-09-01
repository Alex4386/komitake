package main

import (
	"context"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
)

func newStatusCommand(opts *options) *cobra.Command {
	var showSecrets bool

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show daemon mode and pairing state",
		GroupID: groupDaemon,
		Args:    cobra.NoArgs,
		Example: `  komitake status
  komitake status --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withClient(cmd, defaultCallTimeout, func(ctx context.Context, client adminv1.AdminServiceClient) error {
				resp, err := client.GetState(ctx, &adminv1.GetStateRequest{})
				if err != nil {
					return err
				}

				if opts.jsonOutput {
					return writeJSON(cmd, map[string]any{
						"mode":     stateToMode(resp.State),
						"state":    resp.State.String(),
						"pairing":  resp.Pairing,
						"wireless": resp.Wireless,
					})
				}

				u := opts.ui
				mode := stateToMode(resp.State)

				// Plain mode name on the first line keeps `komitake status` usable
				// in scripts and conditionals.
				u.Println(u.paint(modeColor(u, mode), mode))

				if w := resp.Wireless; w != nil {
					if w.Interface != "" {
						u.Field(9, "interface", "%s", w.Interface)
					}
					if w.Address != "" {
						u.Field(9, "address", "%s", w.Address)
					}
					if w.Subnet != "" {
						u.Field(9, "subnet", "%s", w.Subnet)
					}
					if w.HostapdPath != "" {
						u.Field(9, "hostapd", "%s", w.HostapdPath)
					}
				}

				if p := resp.Pairing; p != nil {
					u.Field(9, "ssid", "%s", p.Ssid)
					if p.Channel != 0 {
						u.Field(9, "channel", "%d", p.Channel)
					}
					if showSecrets {
						u.Field(9, "seed", "%s", p.SeedHex)
					} else {
						u.Field(9, "seed", "%s", u.paint(u.c.dim, "hidden (--show-secrets to reveal)"))
					}
				} else if w := resp.Wireless; w != nil {
					if w.Ssid != "" {
						u.Field(9, "ssid", "%s", w.Ssid)
					}
					if w.Channel != 0 {
						u.Field(9, "channel", "%d", w.Channel)
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "print the pairing seed, which is equivalent to the network key")
	return cmd
}

func modeColor(u *ui, mode string) string {
	switch mode {
	case "normal":
		return u.c.green
	case "pairing":
		return u.c.yellow
	case "stopped":
		return u.c.red
	default:
		return ""
	}
}
