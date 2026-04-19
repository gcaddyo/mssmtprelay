package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"

	"localrelay/internal/config"
	"localrelay/internal/storage"
)

var ErrInteractionRequired = errors.New("interactive login required")

// defaultScopes are delegated Graph API scopes requested by this app.
// Note: MSAL Go automatically appends OpenID reserved scopes (openid/profile/offline_access),
// so we intentionally don't include offline_access here to avoid false "declined scope" failures.
var defaultScopes = []string{"User.Read", "Mail.Send", "Mail.Send.Shared"}

// Manager encapsulates MSAL public client auth and persistent token cache.
type Manager struct {
	cfg    config.Config
	store  *storage.Store
	logger *slog.Logger
	app    public.Client
}

// NewManager builds a device-code capable MSAL client with file-backed cache.
func NewManager(cfg config.Config, store *storage.Store, logger *slog.Logger) (*Manager, error) {
	if err := cfg.ValidateAuthConfig(); err != nil {
		return nil, err
	}
	cacheAccessor := &fileCacheAccessor{store: store}
	authority := fmt.Sprintf("https://login.microsoftonline.com/%s", cfg.TenantID)
	app, err := public.New(cfg.ClientID,
		public.WithAuthority(authority),
		public.WithCache(cacheAccessor),
	)
	if err != nil {
		return nil, fmt.Errorf("create public client: %w", err)
	}
	return &Manager{cfg: cfg, store: store, logger: logger, app: app}, nil
}

func (m *Manager) Scopes() []string {
	cp := make([]string, len(defaultScopes))
	copy(cp, defaultScopes)
	return cp
}

// AcquireTokenSilent tries cached account tokens without interactive prompts.
func (m *Manager) AcquireTokenSilent(ctx context.Context, preferredAccountID string) (public.AuthResult, error) {
	accounts, err := m.app.Accounts(ctx)
	if err != nil {
		return public.AuthResult{}, fmt.Errorf("list cached accounts: %w", err)
	}
	if len(accounts) == 0 {
		return public.AuthResult{}, ErrInteractionRequired
	}

	selected := accounts[0]
	if preferredAccountID != "" {
		for _, acc := range accounts {
			if strings.EqualFold(acc.HomeAccountID, preferredAccountID) {
				selected = acc
				break
			}
		}
	}

	res, err := m.app.AcquireTokenSilent(ctx, m.Scopes(), public.WithSilentAccount(selected))
	if err != nil {
		return public.AuthResult{}, fmt.Errorf("%w: %v", ErrInteractionRequired, err)
	}
	return res, nil
}

// AcquireTokenByDeviceCode starts device code flow and waits for browser login.
func (m *Manager) AcquireTokenByDeviceCode(ctx context.Context) (public.AuthResult, error) {
	deviceCode, err := m.app.AcquireTokenByDeviceCode(ctx, m.Scopes())
	if err != nil {
		return public.AuthResult{}, fmt.Errorf("start device code flow: %w", err)
	}
	if m.logger != nil {
		m.logger.Info("Complete device code login in your browser", "message", deviceCode.Result.Message)
	}
	fmt.Println(deviceCode.Result.Message)

	res, err := deviceCode.AuthenticationResult(ctx)
	if err != nil {
		return public.AuthResult{}, fmt.Errorf("device code authentication failed: %w", err)
	}
	return res, nil
}

// AcquireToken prefers silent refresh and falls back to device-code when allowed.
func (m *Manager) AcquireToken(ctx context.Context, preferredAccountID string, allowInteractive bool) (public.AuthResult, error) {
	res, err := m.AcquireTokenSilent(ctx, preferredAccountID)
	if err == nil {
		return res, nil
	}
	if !allowInteractive {
		if errors.Is(err, ErrInteractionRequired) {
			return public.AuthResult{}, err
		}
		return public.AuthResult{}, fmt.Errorf("silent token acquisition failed: %w", err)
	}
	return m.AcquireTokenByDeviceCode(ctx)
}

type fileCacheAccessor struct {
	store *storage.Store
}

func (f *fileCacheAccessor) Replace(ctx context.Context, um cache.Unmarshaler, hints cache.ReplaceHints) error {
	// Restore cache blob before MSAL token lookup.
	data, err := f.store.LoadTokenCache()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return um.Unmarshal(data)
}

func (f *fileCacheAccessor) Export(ctx context.Context, m cache.Marshaler, hints cache.ExportHints) error {
	// Persist cache blob after MSAL token updates.
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	return f.store.SaveTokenCache(data)
}
