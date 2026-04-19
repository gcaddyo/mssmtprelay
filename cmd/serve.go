package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"localrelay/internal/auth"
	"localrelay/internal/graph"
	"localrelay/internal/smtprelay"
	"localrelay/internal/util"
)

// newServeCmd starts local SMTP relay and forwards accepted mail to Graph.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the local SMTP relay service",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			binding, err := loadBindingOrErr(app.Store)
			if err != nil {
				return err
			}

			authCfg := app.Cfg
			if strings.TrimSpace(authCfg.TenantID) == "" {
				authCfg.TenantID = binding.TenantID
			}
			if strings.TrimSpace(authCfg.ClientID) == "" {
				authCfg.ClientID = binding.ClientID
			}
			if err := authCfg.ValidateAuthConfig(); err != nil {
				return err
			}

			tlsCfg, err := util.LoadServerTLSConfig(app.Cfg.TLSCertFile, app.Cfg.TLSKeyFile, app.Cfg.TLSMinVersion)
			if err != nil {
				return err
			}

			manager, err := auth.NewManager(authCfg, app.Store, app.Logger)
			if err != nil {
				return err
			}
			tokens := auth.NewTokenProvider(manager, binding)
			graphClient := graph.NewClient(tokens, app.Logger)

			me, err := graphClient.GetMe(context.Background(), false)
			if err != nil {
				return fmt.Errorf("failed to validate bound account token: %w", err)
			}
			if err := ensureBoundAccountMatches(binding, me.UserPrincipalName, me.Mail); err != nil {
				return err
			}

			sender := &graphSender{client: graphClient}
			relay, err := smtprelay.New(smtprelay.RelayConfig{
				BindAddr:          app.Cfg.SMTPBindAddr,
				EnableSMTPS:       app.Cfg.EnableSMTPS,
				SMTPSBindAddr:     app.Cfg.SMTPSBindAddr,
				TLSConfig:         tlsCfg,
				SMTPUsername:      binding.SMTPUsername,
				PasswordHash:      binding.SMTPPasswordHash,
				FromAddress:       binding.FromAddress(),
				AllowHTML:         app.Cfg.AllowHTML,
				AllowInsecureAuth: app.Cfg.AllowInsecureAuth,
			}, sender, app.Logger)
			if err != nil {
				return err
			}

			fmt.Printf("SMTP relay listening on %s\n", app.Cfg.SMTPBindAddr)
			fmt.Printf("Bound account: %s\n", binding.FromAddress())
			if app.Cfg.AllowInsecureAuth {
				fmt.Printf("TLS mode: STARTTLS optional for AUTH (unsafe)\n")
				fmt.Printf("WARNING: insecure mode enabled, AUTH without TLS is allowed\n")
			} else {
				fmt.Printf("TLS mode: STARTTLS required for AUTH\n")
			}
			if app.Cfg.EnableSMTPS {
				fmt.Printf("SMTPS listener: %s\n", app.Cfg.SMTPSBindAddr)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return relay.Serve(ctx)
		},
	}
}

type graphSender struct {
	client *graph.Client
}

// Send translates SMTP payload into Graph sendMail payload.
func (g *graphSender) Send(ctx context.Context, message smtprelay.OutboundMessage) error {
	recipients := make([]graph.Recipient, 0, len(message.Recipients))
	for _, r := range message.Recipients {
		recipients = append(recipients, graph.Recipient{Address: r})
	}
	return g.client.SendMail(ctx, graph.SendMailInput{
		Subject:     message.Subject,
		TextBody:    message.TextBody,
		HTMLBody:    message.HTMLBody,
		UseHTML:     message.UseHTML,
		Recipients:  recipients,
		FromAddress: message.FromAddress,
	}, false)
}
