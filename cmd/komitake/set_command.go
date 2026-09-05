package main

import (
	"fmt"
	"strings"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/spf13/cobra"
)

// boolFlag implements pflag.Value for tri-state booleans (unset / true / false).
type boolFlag struct {
	value bool
	set   bool
}

func (flag *boolFlag) Set(raw string) error {
	flag.set = true
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		flag.value = true
	case "false", "0", "no", "off":
		flag.value = false
	default:
		return fmt.Errorf("want true or false")
	}
	return nil
}

func (flag *boolFlag) String() string {
	if !flag.set {
		return ""
	}
	if flag.value {
		return "true"
	}
	return "false"
}

func (flag *boolFlag) Type() string { return "bool" }

func newSetCommand(opts *options) *cobra.Command {
	var (
		webBind             string
		webTLSEnabled       boolFlag
		webTLSCertFile      string
		webTLSKeyFile       string
		webAllowConfig      boolFlag
		socketBind          string
		socketChmod         string
		wirelessInterface   string
		wirelessAddress     string
		wirelessChannel     string
		wirelessHostapdPath string
		wirelessSSID        string
		wirelessPSK         string
		pairingFile         string
		autostart           boolFlag
		videoHwaccel        string
		videoFFmpegPath     string
		videoFFmpegProfile  string
		rcdName             string
		secret              string
		generateSecret      bool
	)

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Change Komitake settings",
		Long: "Change specific Komitake settings without editing config.json by hand.\n\n" +
			"Only flags you pass are updated; everything else stays as-is.\n" +
			"Booleans accept true/false, for example --web-tls-enabled=false.\n\n" +
			"Restart the daemon and web UI to apply listener or wireless changes.",
		GroupID: groupConfig,
		Args:    cobra.NoArgs,
		Example: `  komitake set --wireless-interface=wlan1
  komitake set --web-bind=0.0.0.0:8080 --socket-chmod=0770
  komitake set --web-tls-enabled=true
  komitake set --video-hwaccel=vaapi --video-ffmpeg-profile=realtime
  komitake set --generate-secret
  komitake set --secret='replace-with-a-stable-secret'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("generate-secret") {
				generated, err := config.GenerateRootSecret()
				if err != nil {
					return err
				}
				secret = generated
			}
			changes, err := collectSettingsChanges(cmd, settingsFlagBundle{
				webBind:             &webBind,
				webTLSEnabled:       &webTLSEnabled,
				webTLSCertFile:      &webTLSCertFile,
				webTLSKeyFile:       &webTLSKeyFile,
				webAllowConfig:      &webAllowConfig,
				socketBind:          &socketBind,
				socketChmod:         &socketChmod,
				wirelessInterface:   &wirelessInterface,
				wirelessAddress:     &wirelessAddress,
				wirelessChannel:     &wirelessChannel,
				wirelessHostapdPath: &wirelessHostapdPath,
				wirelessSSID:        &wirelessSSID,
				wirelessPSK:         &wirelessPSK,
				pairingFile:         &pairingFile,
				autostart:           &autostart,
				videoHwaccel:        &videoHwaccel,
				videoFFmpegPath:     &videoFFmpegPath,
				videoFFmpegProfile:  &videoFFmpegProfile,
				rcdName:             &rcdName,
				secret:              &secret,
			})
			if err != nil {
				return err
			}
			if err := config.ApplySettingsChanges(opts.configPath, changes); err != nil {
				return err
			}
			path, err := config.ResolveConfigPath(opts.configPath)
			if err != nil {
				return err
			}
			opts.ui.Printf("config saved to %s\n", path)
			if changes.Secret != nil {
				opts.ui.Printf("secret saved to %s\n", config.DefaultSecretFile(path))
			}
			opts.ui.Println("restart komitake daemon and web to apply listener, wireless, or video changes")
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.configPath, "config", "", "path to config.json")
	flags.StringVar(&webBind, "web-bind", "", "web.bind: HTTP listener for the Web UI")
	flags.Var(&webTLSEnabled, "web-tls-enabled", "web.tls.enabled: serve HTTPS")
	flags.StringVar(&webTLSCertFile, "web-tls-cert-file", "", "web.tls.cert_file: PEM certificate chain")
	flags.StringVar(&webTLSKeyFile, "web-tls-key-file", "", "web.tls.key_file: PEM private key")
	flags.Var(&webAllowConfig, "web-allow-config", "web.allow_config: allow editing settings from the Web UI")
	flags.StringVar(&socketBind, "socket-bind", "", "socket.bind: admin API unix:/path or host:port")
	flags.StringVar(&socketChmod, "socket-chmod", "", "socket.chmod: unix socket mode (octal)")
	flags.StringVar(&wirelessInterface, "wireless-interface", "", "wireless.interface: Wi-Fi adapter for the kart AP")
	flags.StringVar(&wirelessAddress, "wireless-address", "", "wireless.address: AP host address with prefix (e.g. 192.168.137.1/24)")
	flags.StringVar(&wirelessChannel, "wireless-channel", "", "wireless.channel: Wi-Fi channel")
	flags.StringVar(&wirelessHostapdPath, "wireless-hostapd-path", "", "wireless.hostapd_path: patched hostapd binary")
	flags.StringVar(&wirelessSSID, "wireless-ssid", "", "wireless.ssid: fixed AP SSID (set together with --wireless-psk)")
	flags.StringVar(&wirelessPSK, "wireless-psk", "", "wireless.psk: fixed AP PSK as hex (set together with --wireless-ssid)")
	flags.StringVar(&pairingFile, "pairing-file", "", "pairing_file: pairing session store path")
	flags.Var(&autostart, "autostart", "autostart: start the AP when the daemon boots")
	flags.StringVar(&videoHwaccel, "video-hwaccel", "", "video.hwaccel: auto, vaapi, nvenc, qsv, custom, or none (software/libx264)")
	flags.StringVar(&videoFFmpegPath, "video-ffmpeg-path", "", "video.ffmpeg_path: ffmpeg binary path")
	flags.StringVar(&videoFFmpegProfile, "video-ffmpeg-profile", "", "video.ffmpeg_profile: realtime (low latency), or empty to clear")
	flags.StringVar(&rcdName, "rcd-name", "", "rcd.name: server display name shown to karts")
	flags.StringVar(&secret, "secret", "", "root secret written to the sibling secret file (not config.json)")
	flags.BoolVar(&generateSecret, "generate-secret", false, "generate a random root secret and write it to the sibling secret file")

	cmd.MarkFlagsMutuallyExclusive("secret", "generate-secret")
	_ = cmd.MarkFlagFilename("config", "json")
	_ = cmd.RegisterFlagCompletionFunc("video-hwaccel",
		cobra.FixedCompletions([]string{
			config.VideoHwaccelAuto,
			config.VideoHwaccelVAAPI,
			config.VideoHwaccelNVENC,
			config.VideoHwaccelQSV,
			config.VideoHwaccelCustom,
			config.VideoHwaccelNone,
		}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("video-ffmpeg-profile",
		cobra.FixedCompletions([]string{config.VideoFFmpegProfileRealtime}, cobra.ShellCompDirectiveNoFileComp))

	return cmd
}

type settingsFlagBundle struct {
	webBind, webTLSCertFile, webTLSKeyFile                             *string
	socketBind, socketChmod                                            *string
	wirelessInterface, wirelessAddress, wirelessChannel                *string
	wirelessHostapdPath, wirelessSSID, wirelessPSK                     *string
	pairingFile, videoHwaccel, videoFFmpegPath, videoFFmpegProfile, rcdName, secret *string
	webTLSEnabled, autostart, webAllowConfig                                        *boolFlag
}

func collectSettingsChanges(cmd *cobra.Command, flags settingsFlagBundle) (config.SettingsChanges, error) {
	changes := config.SettingsChanges{}
	set := false

	if cmd.Flags().Changed("web-bind") {
		changes.WebBind = flags.webBind
		set = true
	}
	if flags.webTLSEnabled.set {
		value := flags.webTLSEnabled.value
		changes.WebTLSEnabled = &value
		set = true
	}
	if cmd.Flags().Changed("web-tls-cert-file") {
		changes.WebTLSCertFile = flags.webTLSCertFile
		set = true
	}
	if cmd.Flags().Changed("web-tls-key-file") {
		changes.WebTLSKeyFile = flags.webTLSKeyFile
		set = true
	}
	if flags.webAllowConfig.set {
		value := flags.webAllowConfig.value
		changes.WebAllowConfig = &value
		set = true
	}
	if cmd.Flags().Changed("socket-bind") {
		changes.SocketBind = flags.socketBind
		set = true
	}
	if cmd.Flags().Changed("socket-chmod") {
		changes.SocketChmod = flags.socketChmod
		set = true
	}
	if cmd.Flags().Changed("wireless-interface") {
		changes.WirelessInterface = flags.wirelessInterface
		set = true
	}
	if cmd.Flags().Changed("wireless-address") {
		changes.WirelessAddress = flags.wirelessAddress
		set = true
	}
	if cmd.Flags().Changed("wireless-channel") {
		changes.WirelessChannel = flags.wirelessChannel
		set = true
	}
	if cmd.Flags().Changed("wireless-hostapd-path") {
		changes.WirelessHostapdPath = flags.wirelessHostapdPath
		set = true
	}
	if cmd.Flags().Changed("wireless-ssid") {
		changes.WirelessSSID = flags.wirelessSSID
		set = true
	}
	if cmd.Flags().Changed("wireless-psk") {
		changes.WirelessPSK = flags.wirelessPSK
		set = true
	}
	if cmd.Flags().Changed("pairing-file") {
		changes.PairingFile = flags.pairingFile
		set = true
	}
	if flags.autostart.set {
		value := flags.autostart.value
		changes.Autostart = &value
		set = true
	}
	if cmd.Flags().Changed("video-hwaccel") {
		changes.VideoHwaccel = flags.videoHwaccel
		set = true
	}
	if cmd.Flags().Changed("video-ffmpeg-path") {
		changes.VideoFFmpegPath = flags.videoFFmpegPath
		set = true
	}
	if cmd.Flags().Changed("video-ffmpeg-profile") {
		changes.VideoFFmpegProfile = flags.videoFFmpegProfile
		set = true
	}
	if cmd.Flags().Changed("rcd-name") {
		changes.RCDName = flags.rcdName
		set = true
	}
	if cmd.Flags().Changed("secret") || cmd.Flags().Changed("generate-secret") {
		changes.Secret = flags.secret
		set = true
	}

	if !set {
		return config.SettingsChanges{}, fmt.Errorf("no settings specified; run komitake set --help")
	}
	return changes, nil
}
