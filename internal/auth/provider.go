package auth

import (
	"context"
	"errors"
	"fmt"

	"localrelay/internal/storage"
)

type TokenProvider struct {
	manager          *Manager
	preferredAccount string
}

// NewTokenProvider creates a Graph token source pinned to bound account metadata.
func NewTokenProvider(manager *Manager, binding *storage.Binding) *TokenProvider {
	preferred := ""
	if binding != nil {
		preferred = binding.AuthAccountID
	}
	return &TokenProvider{manager: manager, preferredAccount: preferred}
}

// GetAccessToken returns delegated access token and normalizes interactive errors.
func (p *TokenProvider) GetAccessToken(ctx context.Context, allowInteractive bool) (string, error) {
	res, err := p.manager.AcquireToken(ctx, p.preferredAccount, allowInteractive)
	if err != nil {
		if errors.Is(err, ErrInteractionRequired) {
			return "", fmt.Errorf("token refresh requires interactive login; run bind again")
		}
		return "", err
	}
	return res.AccessToken, nil
}
