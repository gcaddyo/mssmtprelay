package util

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ParseTLSMinVersion converts config value to tls.Version* constants.
func ParseTLSMinVersion(v string) (uint16, error) {
	switch strings.TrimSpace(v) {
	case "1.2", "tls1.2", "TLS1.2", "TLS12", "tls12", "":
		return tls.VersionTLS12, nil
	case "1.3", "tls1.3", "TLS1.3", "TLS13", "tls13":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported TLS minimum version: %s", v)
	}
}

// LoadServerTLSConfig validates cert/key presence and builds strict server TLS config.
func LoadServerTLSConfig(certFile, keyFile, minVersion string) (*tls.Config, error) {
	if strings.TrimSpace(certFile) == "" {
		return nil, errors.New("TLS_CERT_FILE is required")
	}
	if strings.TrimSpace(keyFile) == "" {
		return nil, errors.New("TLS_KEY_FILE is required")
	}
	if _, err := os.Stat(certFile); err != nil {
		return nil, fmt.Errorf("TLS cert file error (%s): %w", certFile, err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return nil, fmt.Errorf("TLS key file error (%s): %w", keyFile, err)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS cert/key: %w", err)
	}
	mv, err := ParseTLSMinVersion(minVersion)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   mv,
	}
	return cfg, nil
}
