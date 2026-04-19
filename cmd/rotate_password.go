package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"localrelay/internal/util"
)

// newRotatePasswordCmd rotates local SMTP auth password and shows plaintext once.
func newRotatePasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate-password",
		Short: "Rotate local SMTP relay password and show it once",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			binding, err := loadBindingOrErr(app.Store)
			if err != nil {
				return err
			}

			password, err := util.RandomString(24)
			if err != nil {
				return err
			}
			hash, err := util.HashPassword(password)
			if err != nil {
				return err
			}
			binding.SMTPPasswordHash = hash
			if err := app.Store.SaveBinding(binding); err != nil {
				return err
			}

			fmt.Println("SMTP relay password rotated successfully.")
			fmt.Printf("SMTP username: %s\n", binding.SMTPUsername)
			fmt.Printf("SMTP password: %s\n", password)
			fmt.Println("Old password is invalid now. This plaintext password is shown only once.")
			return nil
		},
	}
}
