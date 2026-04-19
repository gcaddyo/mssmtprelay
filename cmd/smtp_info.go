package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"localrelay/internal/storage"
)

// newSMTPInfoCmd prints local relay connection info without exposing plaintext password.
func newSMTPInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smtp-info",
		Short: "Show local SMTP relay connection info",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			binding, err := app.Store.LoadBinding()
			if err != nil {
				if errors.Is(err, storage.ErrNotBound) {
					fmt.Println("Bound: no")
					fmt.Println("Run `bind` first to generate local SMTP relay credentials.")
					return nil
				}
				return err
			}

			host, port := displayHostPort(app.Cfg.SMTPBindAddr)
			fmt.Printf("Bound: yes\n")
			fmt.Printf("Bound account: %s\n", binding.FromAddress())
			fmt.Printf("SMTP host: %s\n", host)
			fmt.Printf("SMTP port: %s\n", port)
			fmt.Printf("SMTP username: %s\n", binding.SMTPUsername)
			fmt.Printf("SMTP password: (hidden)\n")
			if app.Cfg.AllowInsecureAuth {
				fmt.Printf("TLS: STARTTLS available, AUTH allowed without TLS (unsafe)\n")
			} else {
				fmt.Printf("TLS: STARTTLS required for AUTH\n")
			}
			fmt.Printf("TLS cert file: %s\n", app.Cfg.TLSCertFile)
			fmt.Printf("TLS key file: %s\n", app.Cfg.TLSKeyFile)
			fmt.Printf("TLS min version: %s\n", app.Cfg.TLSMinVersion)
			fmt.Printf("TLS mode: STARTTLS\n")
			if app.Cfg.EnableSMTPS {
				fmt.Printf("SMTPS enabled: yes (%s)\n", app.Cfg.SMTPSBindAddr)
			} else {
				fmt.Printf("SMTPS enabled: no\n")
			}
			fmt.Println("These credentials are only for connecting to this local relay service.")
			fmt.Println("Outbound mail still goes to Microsoft Graph /me/sendMail.")
			return nil
		},
	}
}
