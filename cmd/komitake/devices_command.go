package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/Alex4386/komitake/internal/deviceselect"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
)

func newDevicesCommand(opts *options) *cobra.Command {
	var long bool

	cmd := &cobra.Command{
		Use:     "devices [selector]",
		Aliases: []string{"ls", "list"},
		Short:   "List connected karts, or show one by ident/serial",
		GroupID: groupDevice,
		Args:    cobra.MaximumNArgs(1),
		Example: `  komitake devices
  komitake devices -l
  komitake devices XKW123
  komitake devices aabbccddeeff0011
  komitake devices --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withClient(cmd, defaultCallTimeout, func(ctx context.Context, client adminv1.AdminServiceClient) error {
				resp, err := client.ListDevices(ctx, &adminv1.ListDevicesRequest{})
				if err != nil {
					return err
				}

				if len(args) == 1 {
					return showDevice(cmd, opts, resp.Devices, args[0])
				}
				return listDevices(cmd, opts, resp.Devices, long)
			})
		},
	}

	cmd.Flags().BoolVarP(&long, "long", "l", false, "show ident, address, MAC, and signal")
	return cmd
}

func listDevices(cmd *cobra.Command, opts *options, devices []*adminv1.DeviceSummary, long bool) error {
	if opts.jsonOutput {
		return writeJSON(cmd, &adminv1.ListDevicesResponse{Devices: devices})
	}

	u := opts.ui
	if len(devices) == 0 {
		u.Println("no devices connected")
		if u.tty {
			u.Warnf("power-cycle the kart to reconnect, or run `komitake pair` if it was never paired")
		}
		return nil
	}

	var t *table
	if long {
		t = newTable("serial", "ident", "kind", "address", "mac", "signal")
	} else {
		t = newTable("serial", "kind")
	}

	for _, d := range devices {
		serial := emptyDash(d.GetSerial())
		if long {
			t.Add(serial, d.Ident, d.Kind, d.Address, d.MacAddress, signalField(d))
			continue
		}
		t.Add(serial, d.Kind)
	}
	t.Render(u)
	return nil
}

func showDevice(cmd *cobra.Command, opts *options, devices []*adminv1.DeviceSummary, selector string) error {
	devs := make([]deviceselect.Device, 0, len(devices))
	byIdent := make(map[string]*adminv1.DeviceSummary, len(devices))
	for _, d := range devices {
		devs = append(devs, deviceselect.Device{
			Ident:      d.Ident,
			Serial:     d.GetSerial(),
			Kind:       d.Kind,
			Address:    d.Address,
			MACAddress: d.MacAddress,
		})
		byIdent[d.Ident] = d
	}

	match, err := deviceselect.Resolve(selector, devs)
	if err != nil {
		return err
	}
	d := byIdent[match.Ident]
	if d == nil {
		return fmt.Errorf("no device matches %q", selector)
	}

	if opts.jsonOutput {
		return writeJSON(cmd, d)
	}

	u := opts.ui
	u.Printf("kind:    %s\n", d.Kind)
	u.Printf("ident:   %s\n", d.Ident)
	u.Printf("serial:  %s\n", emptyDash(d.GetSerial()))
	u.Printf("address: %s\n", emptyDash(d.Address))
	u.Printf("mac:     %s\n", emptyDash(d.MacAddress))
	u.Printf("signal:  %s\n", signalField(d))
	return nil
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// signalField renders the station signal, or "-" when the AP reports none.
func signalField(d *adminv1.DeviceSummary) string {
	if d.SignalDbm == nil {
		return "-"
	}
	return fmt.Sprintf("%d dBm", d.GetSignalDbm())
}
