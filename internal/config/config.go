package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultDataDir       = "./data"
	defaultSMTPBindAddr  = "0.0.0.0:2525"
	defaultSMTPSBindAddr = "0.0.0.0:2465"
	defaultTLSCertFile   = "/certs/server.crt"
	defaultTLSKeyFile    = "/certs/server.key"
	defaultTLSMinVersion = "1.2"
	defaultLogLevel      = "info"
)

// Config holds merged application settings from file/env/flags.
type Config struct {
	TenantID          string `yaml:"tenant_id"`
	ClientID          string `yaml:"client_id"`
	SMTPBindAddr      string `yaml:"smtp_bind_addr"`
	SMTPSBindAddr     string `yaml:"smtps_bind_addr"`
	EnableSMTPS       bool   `yaml:"enable_smtps"`
	DataDir           string `yaml:"data_dir"`
	LogLevel          string `yaml:"log_level"`
	TLSCertFile       string `yaml:"tls_cert_file"`
	TLSKeyFile        string `yaml:"tls_key_file"`
	TLSMinVersion     string `yaml:"tls_min_version"`
	AllowHTML         bool   `yaml:"allow_html"`
	AllowInsecureAuth bool   `yaml:"allow_insecure_auth"`

	ConfigFile string `yaml:"-"`
}

// Overrides represents CLI-level explicit flags and has top priority.
type Overrides struct {
	ConfigFile        *string
	TenantID          *string
	ClientID          *string
	SMTPBindAddr      *string
	SMTPSBindAddr     *string
	EnableSMTPS       *bool
	DataDir           *string
	LogLevel          *string
	TLSCertFile       *string
	TLSKeyFile        *string
	TLSMinVersion     *string
	AllowHTML         *bool
	AllowInsecureAuth *bool
}

func defaultConfig() Config {
	return Config{
		SMTPBindAddr:      defaultSMTPBindAddr,
		SMTPSBindAddr:     defaultSMTPSBindAddr,
		EnableSMTPS:       false,
		DataDir:           defaultDataDir,
		LogLevel:          defaultLogLevel,
		TLSCertFile:       defaultTLSCertFile,
		TLSKeyFile:        defaultTLSKeyFile,
		TLSMinVersion:     defaultTLSMinVersion,
		AllowHTML:         true,
		AllowInsecureAuth: false,
	}
}

// Load merges default config with file, env and CLI overrides in priority order.
func Load(overrides Overrides) (Config, error) {
	cfg := defaultConfig()

	preDataDir := pickString(overrides.DataDir, "DATA_DIR", cfg.DataDir)
	configPath := firstNonEmpty(ptrString(overrides.ConfigFile), os.Getenv("CONFIG_FILE"))
	if configPath == "" {
		configPath = filepath.Join(preDataDir, "config.yaml")
	}
	cfg.ConfigFile = configPath

	if err := mergeFromFile(&cfg, configPath); err != nil {
		return Config{}, err
	}

	mergeFromEnv(&cfg)
	mergeFromOverrides(&cfg, overrides)

	if cfg.DataDir == "" {
		return Config{}, errors.New("data_dir cannot be empty")
	}
	cfg.DataDir = filepath.Clean(cfg.DataDir)
	if cfg.SMTPBindAddr == "" {
		return Config{}, errors.New("smtp_bind_addr cannot be empty")
	}
	if cfg.TLSMinVersion == "" {
		cfg.TLSMinVersion = defaultTLSMinVersion
	}
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}

	return cfg, nil
}

// Save writes effective runtime config to disk with restrictive permissions.
func Save(cfg Config) error {
	if cfg.ConfigFile == "" {
		return errors.New("config file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ConfigFile), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	buf, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(cfg.ConfigFile, buf, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func mergeFromFile(cfg *Config, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("config file path %s is a directory", path)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(buf))) == 0 {
		return nil
	}
	if err := yaml.Unmarshal(buf, cfg); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}

func mergeFromEnv(cfg *Config) {
	if v := os.Getenv("TENANT_ID"); v != "" {
		cfg.TenantID = v
	}
	if v := os.Getenv("CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("SMTP_BIND_ADDR"); v != "" {
		cfg.SMTPBindAddr = v
	}
	if v := os.Getenv("SMTPS_BIND_ADDR"); v != "" {
		cfg.SMTPSBindAddr = v
	}
	if v := os.Getenv("ENABLE_SMTPS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.EnableSMTPS = b
		}
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("TLS_MIN_VERSION"); v != "" {
		cfg.TLSMinVersion = v
	}
	if v := os.Getenv("ALLOW_HTML"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AllowHTML = b
		}
	}
	if v := os.Getenv("ALLOW_INSECURE_AUTH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AllowInsecureAuth = b
		}
	}
}

func mergeFromOverrides(cfg *Config, o Overrides) {
	if o.TenantID != nil {
		cfg.TenantID = *o.TenantID
	}
	if o.ClientID != nil {
		cfg.ClientID = *o.ClientID
	}
	if o.SMTPBindAddr != nil {
		cfg.SMTPBindAddr = *o.SMTPBindAddr
	}
	if o.SMTPSBindAddr != nil {
		cfg.SMTPSBindAddr = *o.SMTPSBindAddr
	}
	if o.EnableSMTPS != nil {
		cfg.EnableSMTPS = *o.EnableSMTPS
	}
	if o.DataDir != nil {
		cfg.DataDir = *o.DataDir
	}
	if o.LogLevel != nil {
		cfg.LogLevel = *o.LogLevel
	}
	if o.TLSCertFile != nil {
		cfg.TLSCertFile = *o.TLSCertFile
	}
	if o.TLSKeyFile != nil {
		cfg.TLSKeyFile = *o.TLSKeyFile
	}
	if o.TLSMinVersion != nil {
		cfg.TLSMinVersion = *o.TLSMinVersion
	}
	if o.AllowHTML != nil {
		cfg.AllowHTML = *o.AllowHTML
	}
	if o.AllowInsecureAuth != nil {
		cfg.AllowInsecureAuth = *o.AllowInsecureAuth
	}
}

func pickString(cli *string, envKey string, fallback string) string {
	if cli != nil {
		return *cli
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fallback
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ValidateAuthConfig ensures required Entra app identifiers are present.
func (c Config) ValidateAuthConfig() error {
	if strings.TrimSpace(c.TenantID) == "" {
		return errors.New("TENANT_ID is required")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("CLIENT_ID is required")
	}
	return nil
}
