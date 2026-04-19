package smtprelay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	smtp "github.com/emersion/go-smtp"

	"localrelay/internal/graph"
	"localrelay/internal/util"
)

const forwardTimeout = 60 * time.Second

type OutboundMessage struct {
	Subject     string
	TextBody    string
	HTMLBody    string
	UseHTML     bool
	Recipients  []string
	FromAddress string
}

// Sender is the outbound delivery abstraction used by SMTP layer.
// The current implementation sends to Microsoft Graph.
type Sender interface {
	Send(ctx context.Context, message OutboundMessage) error
}

// RelayConfig configures local SMTP listeners and auth policy.
type RelayConfig struct {
	BindAddr          string
	EnableSMTPS       bool
	SMTPSBindAddr     string
	TLSConfig         *tls.Config
	SMTPUsername      string
	PasswordHash      string
	FromAddress       string
	AllowHTML         bool
	AllowInsecureAuth bool
}

// Relay hosts STARTTLS SMTP (and optional SMTPS) endpoints.
type Relay struct {
	logger         *slog.Logger
	startTLSServer *smtp.Server
	smtpsServer    *smtp.Server
}

// New creates the SMTP relay server with strict TLS-before-AUTH policy.
func New(cfg RelayConfig, sender Sender, logger *slog.Logger) (*Relay, error) {
	if sender == nil {
		return nil, errors.New("sender is nil")
	}
	if cfg.TLSConfig == nil {
		return nil, errors.New("TLS config is required")
	}
	if strings.TrimSpace(cfg.SMTPUsername) == "" || strings.TrimSpace(cfg.PasswordHash) == "" {
		return nil, errors.New("SMTP credentials are missing")
	}
	if strings.TrimSpace(cfg.BindAddr) == "" {
		cfg.BindAddr = "0.0.0.0:2525"
	}

	be := &backend{
		sender:            sender,
		smtpUsername:      cfg.SMTPUsername,
		passwordHash:      cfg.PasswordHash,
		fromAddress:       cfg.FromAddress,
		allowHTML:         cfg.AllowHTML,
		allowInsecureAuth: cfg.AllowInsecureAuth,
		logger:            logger,
	}

	starttls := smtp.NewServer(be)
	starttls.Addr = cfg.BindAddr
	starttls.Domain = "localhost"
	starttls.AllowInsecureAuth = cfg.AllowInsecureAuth
	starttls.TLSConfig = cfg.TLSConfig
	starttls.MaxRecipients = 200
	starttls.MaxMessageBytes = 20 * 1024 * 1024
	starttls.ReadTimeout = 60 * time.Second
	starttls.WriteTimeout = 60 * time.Second
	if logger != nil {
		starttls.ErrorLog = log.New(&slogWriter{logger: logger.With("component", "smtp-server")}, "", 0)
	}

	r := &Relay{logger: logger, startTLSServer: starttls}
	if cfg.EnableSMTPS {
		smtps := smtp.NewServer(be)
		smtps.Addr = cfg.SMTPSBindAddr
		smtps.Domain = "localhost"
		smtps.AllowInsecureAuth = cfg.AllowInsecureAuth
		smtps.TLSConfig = cfg.TLSConfig
		smtps.MaxRecipients = starttls.MaxRecipients
		smtps.MaxMessageBytes = starttls.MaxMessageBytes
		smtps.ReadTimeout = starttls.ReadTimeout
		smtps.WriteTimeout = starttls.WriteTimeout
		if logger != nil {
			smtps.ErrorLog = log.New(&slogWriter{logger: logger.With("component", "smtps-server")}, "", 0)
		}
		r.smtpsServer = smtps
	}

	return r, nil
}

// Serve starts listeners and blocks until context cancellation or fatal error.
func (r *Relay) Serve(ctx context.Context) error {
	errCh := make(chan error, 2)

	go func() {
		if r.logger != nil {
			r.logger.Info("SMTP relay (STARTTLS) listening", "addr", r.startTLSServer.Addr)
		}
		if err := r.startTLSServer.ListenAndServe(); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			errCh <- fmt.Errorf("smtp STARTTLS server: %w", err)
		}
	}()

	if r.smtpsServer != nil {
		go func() {
			if r.logger != nil {
				r.logger.Info("SMTP relay (SMTPS) listening", "addr", r.smtpsServer.Addr)
			}
			if err := r.smtpsServer.ListenAndServeTLS(); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
				errCh <- fmt.Errorf("smtp SMTPS server: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		return r.shutdown()
	case err := <-errCh:
		_ = r.shutdown()
		return err
	}
}

// shutdown gracefully closes SMTP listeners.
func (r *Relay) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.startTLSServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
		return err
	}
	if r.smtpsServer != nil {
		if err := r.smtpsServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			return err
		}
	}
	return nil
}

type backend struct {
	sender            Sender
	smtpUsername      string
	passwordHash      string
	fromAddress       string
	allowHTML         bool
	allowInsecureAuth bool
	logger            *slog.Logger
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{conn: c, backend: b}, nil
}

type session struct {
	conn          *smtp.Conn
	backend       *backend
	authenticated bool
	mailFrom      string
	rcptTo        []string
}

func (s *session) AuthMechanisms() []string {
	if !s.isTLS() && !s.backend.allowInsecureAuth {
		return nil
	}
	return []string{sasl.Plain, sasl.Login}
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	if !s.isTLS() && !s.backend.allowInsecureAuth {
		return nil, &smtp.SMTPError{Code: 538, EnhancedCode: smtp.EnhancedCode{5, 7, 11}, Message: "Encryption required for requested authentication mechanism"}
	}
	switch strings.ToUpper(mech) {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if s.validateCredentials(username, password) {
				s.authenticated = true
				return nil
			}
			return smtp.ErrAuthFailed
		}), nil
	case sasl.Login:
		return newLoginServer(func(username, password string) error {
			if s.validateCredentials(username, password) {
				s.authenticated = true
				return nil
			}
			return smtp.ErrAuthFailed
		}), nil
	default:
		return nil, smtp.ErrAuthUnknownMechanism
	}
}

func (s *session) Reset() {
	s.mailFrom = ""
	s.rcptTo = nil
}

func (s *session) Logout() error {
	return nil
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if !s.authenticated {
		return smtp.ErrAuthRequired
	}
	s.mailFrom = from
	s.rcptTo = nil
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if !s.authenticated {
		return smtp.ErrAuthRequired
	}
	s.rcptTo = append(s.rcptTo, strings.TrimSpace(to))
	return nil
}

func (s *session) Data(r io.Reader) error {
	if !s.authenticated {
		return smtp.ErrAuthRequired
	}
	if len(s.rcptTo) == 0 {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 5, 1}, Message: "No valid recipients"}
	}

	raw, err := io.ReadAll(io.LimitReader(r, 25*1024*1024))
	if err != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Failed to read message body"}
	}
	// Parse raw RFC822 payload and map text/html body for Graph.
	parsed, err := util.ParseMailData(raw, s.backend.allowHTML)
	if err != nil {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "Invalid email format"}
	}
	msg := OutboundMessage{
		Subject:     parsed.Subject,
		TextBody:    parsed.TextBody,
		HTMLBody:    parsed.HTMLBody,
		UseHTML:     parsed.HasHTML && s.backend.allowHTML,
		Recipients:  append([]string(nil), s.rcptTo...),
		FromAddress: s.resolveFromAddress(),
	}
	if !msg.UseHTML && strings.TrimSpace(msg.TextBody) == "" {
		msg.TextBody = "(empty body)"
	}
	if msg.UseHTML && strings.TrimSpace(msg.HTMLBody) == "" {
		msg.UseHTML = false
		msg.TextBody = "(empty body)"
	}

	sendCtx, cancel := context.WithTimeout(context.Background(), forwardTimeout)
	defer cancel()
	err = s.backend.sender.Send(sendCtx, msg)
	if err != nil {
		if s.backend.logger != nil {
			s.backend.logger.Error("Failed to forward SMTP message to Graph", "error", err, "mail_from", s.mailFrom, "effective_from", msg.FromAddress, "bound_from", s.backend.fromAddress)
		}
		return mapGraphError(err)
	}

	s.Reset()
	return nil
}

func (s *session) validateCredentials(username, password string) bool {
	if strings.TrimSpace(username) != s.backend.smtpUsername {
		return false
	}
	return util.VerifyPassword(s.backend.passwordHash, password)
}

func (s *session) isTLS() bool {
	_, ok := s.conn.TLSConnectionState()
	return ok
}

func (s *session) resolveFromAddress() string {
	bound := normalizeEnvelopeAddress(s.backend.fromAddress)
	requested := normalizeEnvelopeAddress(s.mailFrom)
	if requested == "" {
		return bound
	}
	if requested == bound {
		return bound
	}
	// Use SMTP MAIL FROM only when Graph accepts it (SendAs/SendOnBehalf permissions).
	return requested
}

func normalizeEnvelopeAddress(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || v == "<>" {
		return ""
	}
	if strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">") {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	if parsed, err := mail.ParseAddress(v); err == nil {
		return strings.ToLower(strings.TrimSpace(parsed.Address))
	}
	return strings.ToLower(v)
}

// mapGraphError converts Graph HTTP errors into SMTP status codes.
func mapGraphError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 4, 2}, Message: "Upstream Graph timeout/cancel, retry later"}
	}
	var ge *graph.Error
	if errors.As(err, &ge) {
		switch {
		case ge.StatusCode == 401:
			return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "Authentication with Graph failed; run bind again"}
		case ge.StatusCode == 403 && strings.EqualFold(ge.Code, "ErrorSendAsDenied"):
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "401 permission denied: no SendAs permission for the requested MAIL FROM address"}
		case ge.StatusCode == 403:
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Graph rejected send request (forbidden)"}
		case ge.StatusCode == 429:
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "Graph rate limit exceeded, retry later"}
		case ge.StatusCode >= 500:
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 2}, Message: "Graph temporary server error"}
		default:
			return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "Graph send failed"}
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "interactive login") {
		return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "Token refresh failed; run bind again"}
	}
	return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary relay failure"}
}

type loginServer struct {
	state int
	user  string
	auth  func(username, password string) error
}

// newLoginServer implements AUTH LOGIN because go-sasl only provides PLAIN.
func newLoginServer(auth func(username, password string) error) sasl.Server {
	return &loginServer{auth: auth}
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.state {
	case 0:
		if len(response) == 0 {
			s.state = 1
			return []byte("Username:"), false, nil
		}
		s.user = string(response)
		s.state = 2
		return []byte("Password:"), false, nil
	case 1:
		s.user = string(response)
		s.state = 2
		return []byte("Password:"), false, nil
	case 2:
		if err := s.auth(s.user, string(response)); err != nil {
			return nil, true, err
		}
		return nil, true, nil
	default:
		return nil, true, errors.New("invalid LOGIN auth state")
	}
}

type slogWriter struct {
	logger *slog.Logger
}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	if w.logger != nil {
		w.logger.Error(strings.TrimSpace(string(p)))
	}
	return len(p), nil
}
