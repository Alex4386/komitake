package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/web"
	"github.com/Alex4386/komitake/pkg/komitake"
	"github.com/spf13/cobra"
)

func newWebCommand(opts *options) *cobra.Command {
	webAddr := config.DefaultWebAddr

	cmd := &cobra.Command{
		Use:     "web",
		Short:   "Serve the REST API and web UI",
		GroupID: groupDaemon,
		Args:    cobra.NoArgs,
		Long: "Serves a REST API under /v1 and the bundled web UI, controlling the daemon\n" +
			"over its admin API. Point --address at the daemon and --web-addr at the\n" +
			"interface to serve the UI on.",
		Example: `  komitake web
  komitake web --web-addr 0.0.0.0:8080
  komitake web --address 192.168.1.50:5252 --web-addr :8080`,
		RunE: func(cmd *cobra.Command, args []string) error {
			webSettings, configErr := config.ResolveWebSettings(opts.configPath)
			if configErr != nil {
				return fmt.Errorf("read web settings: %w", configErr)
			}
			if !cmd.Flags().Changed("web-addr") && webSettings.Bind != "" {
				webAddr = webSettings.Bind
			}
			ep, err := resolveEndpoint(opts)
			if err != nil {
				return err
			}

			client, err := komitake.Dial(ep.String())
			if err != nil {
				return err
			}
			defer client.Close()

			handler := web.Handler(client, web.Options{ConfigPath: opts.configPath})
			server := &http.Server{
				Addr:              webAddr,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
			}
			scheme := "http"
			certificateSource := ""
			if webSettings.TLS.Enabled {
				certificate, source, err := loadWebTLSCertificate(webAddr, webSettings.TLS)
				if err != nil {
					return err
				}
				server.TLSConfig = &tls.Config{
					MinVersion:   tls.VersionTLS12,
					Certificates: []tls.Certificate{certificate},
				}
				scheme = "https"
				certificateSource = source
			}

			ln, err := net.Listen("tcp", webAddr)
			if err != nil {
				return fmt.Errorf("cannot listen on %s: %w", webAddr, err)
			}

			logArguments := []any{"addr", ln.Addr().String(), "scheme", scheme, "daemon", ep.String()}
			if certificateSource != "" {
				logArguments = append(logArguments, "certificate", certificateSource)
			}
			opts.log.Info("web server listening", logArguments...)
			opts.ui.Printf("komitake web on %s://%s (daemon %s)\n", scheme, ln.Addr().String(), ep.String())

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				if webSettings.TLS.Enabled {
					errCh <- server.ServeTLS(ln, "", "")
					return
				}
				errCh <- server.Serve(ln)
			}()

			select {
			case <-ctx.Done():
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return server.Shutdown(shutCtx)
			case err := <-errCh:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			}
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "path to config.json (for web, TLS, and socket settings)")
	cmd.Flags().StringVar(&webAddr, "web-addr", config.DefaultWebAddr, "address to serve the web UI and API on")
	_ = cmd.MarkFlagFilename("config", "json")
	return cmd
}
