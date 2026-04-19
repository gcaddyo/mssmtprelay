package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"localrelay/internal/storage"
	"localrelay/internal/util"
)

// newHealthcheckCmd validates minimal runtime readiness for container healthcheck.
func newHealthcheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "healthcheck",
		Short:  "Healthcheck for container runtime",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := loadAppContext()
			if err != nil {
				return err
			}
			if _, err := app.Store.LoadBinding(); err != nil {
				if errors.Is(err, storage.ErrNotBound) {
					return fmt.Errorf("unbound")
				}
				return err
			}
			if _, err := util.LoadServerTLSConfig(app.Cfg.TLSCertFile, app.Cfg.TLSKeyFile, app.Cfg.TLSMinVersion); err != nil {
				return err
			}
			fmt.Println("ok")
			return nil
		},
	}
	return cmd
}
