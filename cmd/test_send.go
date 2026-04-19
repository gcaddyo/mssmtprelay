package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"localrelay/internal/auth"
	"localrelay/internal/graph"
)

// newTestSendCmd sends a direct Graph test mail without SMTP ingestion.
func newTestSendCmd() *cobra.Command {
	var to []string
	var subject string
	var body string
	var html bool

	cmd := &cobra.Command{
		Use:   "test-send",
		Short: "Send a test email directly via Microsoft Graph /me/sendMail",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(to) == 0 {
				return fmt.Errorf("--to is required")
			}
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

			manager, err := auth.NewManager(authCfg, app.Store, app.Logger)
			if err != nil {
				return err
			}
			client := graph.NewClient(auth.NewTokenProvider(manager, binding), app.Logger)

			recipients := make([]graph.Recipient, 0, len(to))
			for _, addr := range to {
				addr = strings.TrimSpace(addr)
				if addr == "" {
					continue
				}
				recipients = append(recipients, graph.Recipient{Address: addr})
			}
			if len(recipients) == 0 {
				return fmt.Errorf("no valid recipients in --to")
			}
			if strings.TrimSpace(subject) == "" {
				subject = "relayctl test-send"
			}
			if strings.TrimSpace(body) == "" {
				body = "hello from local relay test-send"
			}

			err = client.SendMail(context.Background(), graph.SendMailInput{
				Subject:    subject,
				TextBody:   body,
				HTMLBody:   body,
				UseHTML:    html,
				Recipients: recipients,
			}, false)
			if err != nil {
				return err
			}

			fmt.Printf("Graph sendMail success. From: %s\n", binding.FromAddress())
			fmt.Printf("Recipients: %d\n", len(recipients))
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&to, "to", nil, "Recipient email (repeat or comma-separated)")
	cmd.Flags().StringVar(&subject, "subject", "", "Mail subject")
	cmd.Flags().StringVar(&body, "body", "", "Mail body")
	cmd.Flags().BoolVar(&html, "html", false, "Send body as HTML")
	return cmd
}
