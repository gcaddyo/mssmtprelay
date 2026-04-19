package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	bindingFilename    = "binding.json"
	tokenCacheFilename = "token_cache.bin"
)

var ErrNotBound = errors.New("no bound account found")

// TLSSummary stores relay TLS runtime snapshot in binding metadata.
type TLSSummary struct {
	Enabled        bool   `json:"enabled"`
	Mode           string `json:"mode"`
	CertFile       string `json:"cert_file"`
	KeyFile        string `json:"key_file"`
	MinVersion     string `json:"min_version"`
	SMTPSBindAddr  string `json:"smtps_bind_addr,omitempty"`
	SMTPSListening bool   `json:"smtps_listening"`
}

// Binding is persisted single-account state and local SMTP auth metadata.
type Binding struct {
	TenantID               string     `json:"tenant_id"`
	ClientID               string     `json:"client_id"`
	BoundUserPrincipalName string     `json:"bound_user_principal_name"`
	DisplayName            string     `json:"display_name"`
	Mail                   string     `json:"mail"`
	AuthAccountID          string     `json:"auth_account_id,omitempty"`
	AuthPreferredUsername  string     `json:"auth_preferred_username,omitempty"`
	SMTPUsername           string     `json:"smtp_username"`
	SMTPPasswordHash       string     `json:"smtp_password_hash"`
	SMTPBindAddr           string     `json:"smtp_bind_addr"`
	TLS                    TLSSummary `json:"tls"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// FromAddress returns primary send identity derived from binding profile.
func (b Binding) FromAddress() string {
	if b.Mail != "" {
		return b.Mail
	}
	return b.BoundUserPrincipalName
}

type Store struct {
	dataDir          string
	requestedDataDir string
	usesFallback     bool
}

// New initializes persistent storage directory with secure permissions.
func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("data dir is empty")
	}
	requested := filepath.Clean(dataDir)
	if err := ensureWritableDir(requested); err == nil {
		return &Store{dataDir: requested, requestedDataDir: requested}, nil
	} else if !isPermissionError(err) {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Auto-fallback avoids hard failure in read-only /data mounts.
	fallback := filepath.Join(os.TempDir(), "relayctl-data")
	if err := ensureWritableDir(fallback); err != nil {
		return nil, fmt.Errorf("data dir %s is not writable and fallback %s also failed: %w", requested, fallback, err)
	}
	return &Store{
		dataDir:          fallback,
		requestedDataDir: requested,
		usesFallback:     true,
	}, nil
}

// BindingPath returns on-disk binding metadata path.
func (s *Store) BindingPath() string {
	return filepath.Join(s.dataDir, bindingFilename)
}

// TokenCachePath returns on-disk MSAL token cache path.
func (s *Store) TokenCachePath() string {
	return filepath.Join(s.dataDir, tokenCacheFilename)
}

// DataDir returns the effective storage directory currently in use.
func (s *Store) DataDir() string {
	return s.dataDir
}

// RequestedDataDir returns the configured storage directory from config/env/flags.
func (s *Store) RequestedDataDir() string {
	return s.requestedDataDir
}

// UsesFallback reports whether store switched to a writable temp directory.
func (s *Store) UsesFallback() bool {
	return s.usesFallback
}

func (s *Store) LoadBinding() (*Binding, error) {
	buf, err := os.ReadFile(s.BindingPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotBound
		}
		return nil, fmt.Errorf("read binding: %w", err)
	}
	var b Binding
	if err := json.Unmarshal(buf, &b); err != nil {
		return nil, fmt.Errorf("parse binding: %w", err)
	}
	if b.BoundUserPrincipalName == "" || b.SMTPUsername == "" || b.SMTPPasswordHash == "" {
		return nil, errors.New("binding file is invalid or incomplete")
	}
	return &b, nil
}

// SaveBinding atomically stores binding.json and updates timestamps.
func (s *Store) SaveBinding(b *Binding) error {
	if b == nil {
		return errors.New("binding is nil")
	}
	now := time.Now().UTC()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	buf, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal binding: %w", err)
	}
	if err := writeFileAtomic(s.BindingPath(), buf, 0o600); err != nil {
		return fmt.Errorf("save binding: %w", err)
	}
	return nil
}

func (s *Store) DeleteBinding() error {
	err := os.Remove(s.BindingPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete binding: %w", err)
	}
	return nil
}

func (s *Store) LoadTokenCache() ([]byte, error) {
	buf, err := os.ReadFile(s.TokenCachePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read token cache: %w", err)
	}
	return buf, nil
}

// SaveTokenCache atomically stores the serialized MSAL cache blob.
func (s *Store) SaveTokenCache(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if err := writeFileAtomic(s.TokenCachePath(), data, 0o600); err != nil {
		return fmt.Errorf("save token cache: %w", err)
	}
	return nil
}

func (s *Store) DeleteTokenCache() error {
	err := os.Remove(s.TokenCachePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete token cache: %w", err)
	}
	return nil
}

func (s *Store) Reset() error {
	if err := s.DeleteBinding(); err != nil {
		return err
	}
	if err := s.DeleteTokenCache(); err != nil {
		return err
	}
	return nil
}

// writeFileAtomic prevents partial writes when process exits unexpectedly.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".writecheck")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	_ = os.Remove(probe)
	return nil
}

func isPermissionError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "operation not permitted")
}
