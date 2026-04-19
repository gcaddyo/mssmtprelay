package cmd

import (
	"fmt"
	"log/slog"
	"runtime"

	"github.com/spf13/cobra"

	"localrelay/internal/config"
	"localrelay/internal/logging"
	"localrelay/internal/storage"
)

var rootCmd = &cobra.Command{
	Use:   "relayctl",
	Short: "Local SMTP relay to Microsoft Graph sendMail",
}

// Build metadata injected via -ldflags for release artifacts.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var (
	flagConfigFile        string
	flagTenantID          string
	flagClientID          string
	flagSMTPBindAddr      string
	flagSMTPSBindAddr     string
	flagEnableSMTPS       bool
	flagDataDir           string
	flagLogLevel          string
	flagTLSCertFile       string
	flagTLSKeyFile        string
	flagTLSMinVersion     string
	flagAllowHTML         bool
	flagAllowInsecureAuth bool
)

// Execute runs CLI root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&flagConfigFile, "config", "", "Path to config file (default: DATA_DIR/config.yaml)")
	pf.StringVar(&flagTenantID, "tenant-id", "", "Microsoft Entra tenant ID")
	pf.StringVar(&flagClientID, "client-id", "", "Microsoft Entra app client ID")
	pf.StringVar(&flagSMTPBindAddr, "smtp-bind-addr", "", "SMTP bind address")
	pf.StringVar(&flagSMTPSBindAddr, "smtps-bind-addr", "", "SMTPS bind address")
	pf.BoolVar(&flagEnableSMTPS, "enable-smtps", false, "Enable dedicated SMTPS listener")
	pf.StringVar(&flagDataDir, "data-dir", "", "Data directory for binding and token cache")
	pf.StringVar(&flagLogLevel, "log-level", "", "Log level: debug|info|warn|error")
	pf.StringVar(&flagTLSCertFile, "tls-cert-file", "", "TLS cert file path")
	pf.StringVar(&flagTLSKeyFile, "tls-key-file", "", "TLS key file path")
	pf.StringVar(&flagTLSMinVersion, "tls-min-version", "", "Minimum TLS version: 1.2 or 1.3")
	pf.BoolVar(&flagAllowHTML, "allow-html", true, "Allow HTML mail body when provided")
	pf.BoolVar(&flagAllowInsecureAuth, "allow-insecure-auth", false, "Allow SMTP AUTH without TLS (unsafe)")

	rootCmd.AddCommand(
		newBindCmd(),
		newServeCmd(),
		newSMTPInfoCmd(),
		newStatusCmd(),
		newUnbindCmd(),
		newResetCmd(),
		newTestSendCmd(),
		newRotatePasswordCmd(),
		newHealthcheckCmd(),
		newVersionCmd(),
	)
}

type appContext struct {
	Cfg    config.Config
	Logger *slog.Logger
	Store  *storage.Store
}

// loadAppContext resolves config precedence (flag > env > file) and shared deps.
func loadAppContext() (*appContext, error) {
	overrides := config.Overrides{}
	if changed("config") {
		overrides.ConfigFile = &flagConfigFile
	}
	if changed("tenant-id") {
		overrides.TenantID = &flagTenantID
	}
	if changed("client-id") {
		overrides.ClientID = &flagClientID
	}
	if changed("smtp-bind-addr") {
		overrides.SMTPBindAddr = &flagSMTPBindAddr
	}
	if changed("smtps-bind-addr") {
		overrides.SMTPSBindAddr = &flagSMTPSBindAddr
	}
	if changed("enable-smtps") {
		overrides.EnableSMTPS = &flagEnableSMTPS
	}
	if changed("data-dir") {
		overrides.DataDir = &flagDataDir
	}
	if changed("log-level") {
		overrides.LogLevel = &flagLogLevel
	}
	if changed("tls-cert-file") {
		overrides.TLSCertFile = &flagTLSCertFile
	}
	if changed("tls-key-file") {
		overrides.TLSKeyFile = &flagTLSKeyFile
	}
	if changed("tls-min-version") {
		overrides.TLSMinVersion = &flagTLSMinVersion
	}
	if changed("allow-html") {
		overrides.AllowHTML = &flagAllowHTML
	}
	if changed("allow-insecure-auth") {
		overrides.AllowInsecureAuth = &flagAllowInsecureAuth
	}

	cfg, err := config.Load(overrides)
	if err != nil {
		return nil, err
	}
	logger := logging.New(cfg.LogLevel)
	store, err := storage.New(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if store.UsesFallback() {
		logger.Warn("Configured data directory is not writable; switched to fallback temp directory",
			"requested_data_dir", store.RequestedDataDir(),
			"effective_data_dir", store.DataDir(),
		)
	}
	return &appContext{Cfg: cfg, Logger: logger, Store: store}, nil
}

// changed returns true when a persistent flag is explicitly provided.
func changed(name string) bool {
	f := rootCmd.PersistentFlags().Lookup(name)
	if f == nil {
		panic(fmt.Sprintf("unknown flag: %s", name))
	}
	return f.Changed
}

func buildInfoString() string {
	return fmt.Sprintf("%s (commit=%s date=%s %s/%s)", version, commit, buildDate, runtime.GOOS, runtime.GOARCH)
}
