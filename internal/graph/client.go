package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://graph.microsoft.com/v1.0"

type TokenProvider interface {
	GetAccessToken(ctx context.Context, allowInteractive bool) (string, error)
}

// Client is a thin Microsoft Graph REST wrapper used by this project.
type Client struct {
	httpClient *http.Client
	tokens     TokenProvider
	logger     *slog.Logger
}

// MeProfile is the subset of /me fields we persist and display.
type MeProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Mail              string `json:"mail"`
}

// Recipient represents a single destination email address.
type Recipient struct {
	Address string
}

// SendMailInput maps SMTP payload into Graph /me/sendMail request fields.
type SendMailInput struct {
	Subject     string
	TextBody    string
	HTMLBody    string
	UseHTML     bool
	Recipients  []Recipient
	FromAddress string
}

// Error captures Graph API error metadata for logging and SMTP mapping.
type Error struct {
	StatusCode      int
	Code            string
	Message         string
	RequestID       string
	ClientRequestID string
	RetryAfter      time.Duration
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("graph request failed: HTTP %d", e.StatusCode)}
	if e.Code != "" {
		parts = append(parts, fmt.Sprintf("code=%s", e.Code))
	}
	if e.Message != "" {
		parts = append(parts, fmt.Sprintf("message=%s", e.Message))
	}
	if e.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request-id=%s", e.RequestID))
	}
	if e.ClientRequestID != "" {
		parts = append(parts, fmt.Sprintf("client-request-id=%s", e.ClientRequestID))
	}
	return strings.Join(parts, " ")
}

func (e *Error) IsUnauthorized() bool { return e != nil && e.StatusCode == http.StatusUnauthorized }
func (e *Error) IsForbidden() bool    { return e != nil && e.StatusCode == http.StatusForbidden }
func (e *Error) IsRateLimited() bool  { return e != nil && e.StatusCode == http.StatusTooManyRequests }
func (e *Error) IsRetriable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

func NewClient(tokens TokenProvider, logger *slog.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     tokens,
		logger:     logger,
	}
}

// GetMe calls GET /me to validate current delegated identity.
func (c *Client) GetMe(ctx context.Context, allowInteractive bool) (MeProfile, error) {
	token, err := c.tokens.GetAccessToken(ctx, allowInteractive)
	if err != nil {
		return MeProfile{}, err
	}
	respBody, _, err := c.doJSON(ctx, http.MethodGet, "/me", nil, token)
	if err != nil {
		return MeProfile{}, err
	}
	var me MeProfile
	if err := json.Unmarshal(respBody, &me); err != nil {
		return MeProfile{}, fmt.Errorf("decode /me response: %w", err)
	}
	return me, nil
}

// SendMail calls POST /me/sendMail with bounded retries for 429/5xx.
func (c *Client) SendMail(ctx context.Context, input SendMailInput, allowInteractive bool) error {
	if len(input.Recipients) == 0 {
		return errors.New("no recipients provided")
	}
	contentType := "Text"
	content := input.TextBody
	if input.UseHTML {
		contentType = "HTML"
		content = input.HTMLBody
	}
	if strings.TrimSpace(content) == "" {
		content = "(empty body)"
	}

	payload := map[string]any{
		"message": map[string]any{
			"subject": input.Subject,
			"body": map[string]any{
				"contentType": contentType,
				"content":     content,
			},
			"toRecipients": toGraphRecipients(input.Recipients),
		},
		"saveToSentItems": true,
	}
	if from := strings.TrimSpace(input.FromAddress); from != "" {
		payload["message"].(map[string]any)["from"] = map[string]any{
			"emailAddress": map[string]any{
				"address": from,
			},
		}
	}

	token, err := c.tokens.GetAccessToken(ctx, allowInteractive)
	if err != nil {
		return err
	}

	var lastErr error
	maxAttempts := 4
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, ge, reqErr := c.doJSON(ctx, http.MethodPost, "/me/sendMail", payload, token)
		if reqErr == nil {
			return nil
		}
		lastErr = reqErr
		if ge == nil || !ge.IsRetriable() || attempt == maxAttempts {
			break
		}
		wait := retryDelay(attempt, ge.RetryAfter)
		if c.logger != nil {
			c.logger.Warn("Graph sendMail temporary failure, will retry", "attempt", attempt, "wait", wait.String(), "status", ge.StatusCode, "code", ge.Code, "request_id", ge.RequestID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, accessToken string) ([]byte, *Error, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	clientReqID := strconv.FormatInt(time.Now().UnixNano(), 10)
	req.Header.Set("client-request-id", clientReqID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respBody, nil, nil
	}

	ge := parseGraphError(resp, respBody, clientReqID)
	if c.logger != nil {
		c.logger.Error("Graph API error",
			"status", ge.StatusCode,
			"code", ge.Code,
			"message", ge.Message,
			"request_id", ge.RequestID,
			"client_request_id", ge.ClientRequestID,
		)
	}
	return nil, ge, ge
}

// toGraphRecipients converts recipient list to Graph emailAddress objects.
func toGraphRecipients(recipients []Recipient) []map[string]any {
	result := make([]map[string]any, 0, len(recipients))
	for _, r := range recipients {
		if strings.TrimSpace(r.Address) == "" {
			continue
		}
		result = append(result, map[string]any{
			"emailAddress": map[string]any{"address": strings.TrimSpace(r.Address)},
		})
	}
	return result
}

// parseGraphError extracts Graph response metadata for troubleshooting.
func parseGraphError(resp *http.Response, body []byte, fallbackClientReqID string) *Error {
	ge := &Error{
		StatusCode:      resp.StatusCode,
		RequestID:       resp.Header.Get("request-id"),
		ClientRequestID: resp.Header.Get("client-request-id"),
	}
	if ge.ClientRequestID == "" {
		ge.ClientRequestID = fallbackClientReqID
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if sec, err := strconv.Atoi(ra); err == nil {
			ge.RetryAfter = time.Duration(sec) * time.Second
		}
	}

	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		ge.Code = payload.Error.Code
		ge.Message = payload.Error.Message
	}
	if ge.Message == "" {
		ge.Message = strings.TrimSpace(string(body))
	}
	return ge
}

// retryDelay applies exponential backoff with an upper bound.
func retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	base := math.Pow(2, float64(attempt-1))
	sec := math.Min(base, 8)
	return time.Duration(sec * float64(time.Second))
}
