package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"localrelay/internal/auth"
	"localrelay/internal/graph"
	"localrelay/internal/storage"
	"localrelay/internal/util"
)

// newBindCmd performs first-time account binding and local SMTP credential generation.
func newBindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bind",
		Short: "Bind a single Microsoft 365 work/school account via device code flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			if err := app.Cfg.ValidateAuthConfig(); err != nil {
				return err
			}

			if _, err := app.Store.LoadBinding(); err == nil {
				return errors.New("an account is already bound; run `unbind` or `reset` before binding a different account")
			} else if !errors.Is(err, storage.ErrNotBound) {
				return err
			}

			manager, err := auth.NewManager(app.Cfg, app.Store, app.Logger)
			if err != nil {
				return err
			}

			ctx := context.Background()
			// Interactive login runs in browser; terminal only shows device-code instruction.
			authRes, err := manager.AcquireTokenByDeviceCode(ctx)
			if err != nil {
				return err
			}

			graphClient := graph.NewClient(staticTokenProvider{token: authRes.AccessToken}, app.Logger)
			// Resolve canonical identity after login and persist it as the only bound account.
			me, err := graphClient.GetMe(ctx, false)
			if err != nil {
				return fmt.Errorf("login succeeded but failed to read /me: %w", err)
			}

			password, err := util.RandomString(24)
			if err != nil {
				return err
			}
			hash, err := util.HashPassword(password)
			if err != nil {
				return err
			}
			username := "localrelay-" + strings.ToLower(shortRand(8))

			tlsMode := "STARTTLS required for AUTH"
			if app.Cfg.AllowInsecureAuth {
				tlsMode = "STARTTLS optional for AUTH (unsafe mode)"
			}

			binding := &storage.Binding{
				TenantID:               app.Cfg.TenantID,
				ClientID:               app.Cfg.ClientID,
				BoundUserPrincipalName: me.UserPrincipalName,
				DisplayName:            me.DisplayName,
				Mail:                   me.Mail,
				AuthAccountID:          auth.AccountIDFromResult(authRes),
				AuthPreferredUsername:  auth.PreferredUsernameFromResult(authRes),
				SMTPUsername:           username,
				SMTPPasswordHash:       hash,
				SMTPBindAddr:           app.Cfg.SMTPBindAddr,
				TLS: storage.TLSSummary{
					Enabled:        true,
					Mode:           tlsMode,
					CertFile:       app.Cfg.TLSCertFile,
					KeyFile:        app.Cfg.TLSKeyFile,
					MinVersion:     app.Cfg.TLSMinVersion,
					SMTPSListening: app.Cfg.EnableSMTPS,
					SMTPSBindAddr:  app.Cfg.SMTPSBindAddr,
				},
			}
			if err := app.Store.SaveBinding(binding); err != nil {
				return err
			}

			if err := ensureBoundAccountMatches(binding, me.UserPrincipalName, me.Mail); err != nil {
				return err
			}
			_ = saveConfigBestEffort(app)

			host, port := displayHostPort(binding.SMTPBindAddr)
			fromAddr := binding.FromAddress()
			fmt.Printf("Bound account: %s\n", fromAddr)
			fmt.Printf("SMTP host: %s\n", host)
			fmt.Printf("SMTP port: %s\n", port)
			fmt.Printf("SMTP username: %s\n", binding.SMTPUsername)
			fmt.Printf("SMTP password: %s\n", password)
			fmt.Printf("TLS: %s\n", binding.TLS.Mode)
			fmt.Printf("From address: %s\n", fromAddr)
			fmt.Println("Note: this SMTP username/password is only for this local relay. Outbound email is sent via Microsoft Graph /me/sendMail.")
			return nil
		},
	}
}

type staticTokenProvider struct {
	token string
}

// staticTokenProvider is used once after device login to call /me with fresh token.
func (s staticTokenProvider) GetAccessToken(ctx context.Context, allowInteractive bool) (string, error) {
	return s.token, nil
}

func shortRand(n int) string {
	v, err := util.RandomString(n)
	if err != nil {
		return "relayusr"
	}
	return strings.ToLower(v)
}
