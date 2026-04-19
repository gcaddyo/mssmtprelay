package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"localrelay/internal/auth"
	"localrelay/internal/storage"
	"localrelay/internal/util"
)

// newStatusCmd prints binding/token/TLS/runtime summaries for troubleshooting.
func newStatusCmd() *cobra.Command {
	var short bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show binding, token, SMTP and TLS status",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}

			binding, err := app.Store.LoadBinding()
			if err != nil {
				if errors.Is(err, storage.ErrNotBound) {
					if short {
						fmt.Println("unbound")
						return errors.New("unbound")
					}
					fmt.Println("Bound: no")
					fmt.Println("Account: (none)")
					fmt.Println("Token status: unavailable (not bound)")
					fmt.Printf("Data dir: %s\n", app.Store.DataDir())
					if app.Store.UsesFallback() {
						fmt.Printf("Data dir fallback: true (configured %s is not writable)\n", app.Store.RequestedDataDir())
					}
					fmt.Printf("SMTP bind: %s\n", app.Cfg.SMTPBindAddr)
					fmt.Printf("TLS: cert=%s key=%s min=%s\n", app.Cfg.TLSCertFile, app.Cfg.TLSKeyFile, app.Cfg.TLSMinVersion)
					return nil
				}
				return err
			}

			authCfg := app.Cfg
			if strings.TrimSpace(authCfg.TenantID) == "" {
				authCfg.TenantID = binding.TenantID
			}
			if strings.TrimSpace(authCfg.ClientID) == "" {
				authCfg.ClientID = binding.ClientID
			}

			tokenStatus := "ok (silent refresh available)"
			tokenHealthy := true
			if err := authCfg.ValidateAuthConfig(); err != nil {
				tokenStatus = "unknown (tenant/client missing)"
				tokenHealthy = false
			} else {
				manager, err := auth.NewManager(authCfg, app.Store, app.Logger)
				if err != nil {
					tokenStatus = fmt.Sprintf("error (%v)", err)
					tokenHealthy = false
				} else {
					tokens := auth.NewTokenProvider(manager, binding)
					if _, err := tokens.GetAccessToken(context.Background(), false); err != nil {
						tokenStatus = fmt.Sprintf("invalid (%v)", err)
						tokenHealthy = false
					}
				}
			}

			tlsStatus := "ok"
			tlsHealthy := true
			if _, err := util.LoadServerTLSConfig(app.Cfg.TLSCertFile, app.Cfg.TLSKeyFile, app.Cfg.TLSMinVersion); err != nil {
				tlsStatus = "invalid: " + err.Error()
				tlsHealthy = false
			}

			if short {
				if !tokenHealthy || !tlsHealthy {
					fmt.Printf("degraded token=%v tls=%v\n", tokenHealthy, tlsHealthy)
					return errors.New("status degraded")
				}
				fmt.Println("ok")
				return nil
			}

			host, port := displayHostPort(app.Cfg.SMTPBindAddr)
			fmt.Println("Bound: yes")
			fmt.Printf("Bound account: %s\n", binding.FromAddress())
			fmt.Printf("Display name: %s\n", fmtMasked(binding.DisplayName))
			fmt.Printf("UPN: %s\n", binding.BoundUserPrincipalName)
			fmt.Printf("Token status: %s\n", tokenStatus)
			fmt.Printf("Data dir: %s\n", app.Store.DataDir())
			if app.Store.UsesFallback() {
				fmt.Printf("Data dir fallback: true (configured %s is not writable)\n", app.Store.RequestedDataDir())
			}
			fmt.Printf("SMTP host: %s\n", host)
			fmt.Printf("SMTP port: %s\n", port)
			fmt.Printf("SMTP username: %s\n", binding.SMTPUsername)
			fmt.Println("SMTP password: (hidden)")
			fmt.Printf("TLS status: %s\n", tlsStatus)
			fmt.Printf("TLS cert file: %s\n", app.Cfg.TLSCertFile)
			fmt.Printf("TLS key file: %s\n", app.Cfg.TLSKeyFile)
			fmt.Printf("TLS min version: %s\n", app.Cfg.TLSMinVersion)
			fmt.Printf("Allow insecure AUTH: %v\n", app.Cfg.AllowInsecureAuth)
			fmt.Printf("SMTPS enabled: %v\n", app.Cfg.EnableSMTPS)
			if app.Cfg.EnableSMTPS {
				fmt.Printf("SMTPS bind: %s\n", app.Cfg.SMTPSBindAddr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&short, "short", false, "Only print compact health status")
	return cmd
}
