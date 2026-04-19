package cmd

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"localrelay/internal/storage"
)

func loadBindingOrErr(store *storage.Store) (*storage.Binding, error) {
	b, err := store.LoadBinding()
	if err != nil {
		if errors.Is(err, storage.ErrNotBound) {
			return nil, errors.New("no bound account found; run `bind` first")
		}
		return nil, err
	}
	return b, nil
}

// displayHostPort normalizes bind address for host-side usage hints.
func displayHostPort(bindAddr string) (host string, port string) {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "127.0.0.1", "2525"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, "::") {
		host = "127.0.0.1"
	}
	return host, port
}

func fmtMasked(value string) string {
	if value == "" {
		return "(empty)"
	}
	return value
}

// ensureBoundAccountMatches prevents accidental cross-account token use.
func ensureBoundAccountMatches(binding *storage.Binding, upn, mail string) error {
	if binding == nil {
		return nil
	}
	incoming := strings.ToLower(strings.TrimSpace(upn))
	bound := strings.ToLower(strings.TrimSpace(binding.BoundUserPrincipalName))
	if incoming == "" && mail != "" {
		incoming = strings.ToLower(strings.TrimSpace(mail))
	}
	if bound != "" && incoming != "" && bound != incoming {
		return fmt.Errorf("logged-in account (%s) does not match bound account (%s)", incoming, bound)
	}
	return nil
}
