package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newUnbindCmd removes persisted binding data and token cache.
func newUnbindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unbind",
		Short: "Remove bound account, token cache and local SMTP credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			if err := app.Store.Reset(); err != nil {
				return err
			}
			fmt.Println("Unbound successfully. Binding, token cache, and local SMTP credentials are removed.")
			return nil
		},
	}
}
