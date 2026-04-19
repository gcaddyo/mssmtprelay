package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newResetCmd is hidden alias of unbind for operator convenience.
func newResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "reset",
		Short:  "Alias of unbind: clear binding, token cache, and local SMTP credentials",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			if err := app.Store.Reset(); err != nil {
				return err
			}
			fmt.Println("Reset completed. Binding, token cache, and local SMTP credentials are removed.")
			return nil
		},
	}
}
