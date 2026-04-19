package cmd

import (
	"localrelay/internal/config"
)

// saveConfigBestEffort writes effective config to disk for later non-flag runs.
func saveConfigBestEffort(app *appContext) error {
	cfg := app.Cfg
	if cfg.ConfigFile == "" {
		return nil
	}
	return config.Save(cfg)
}
