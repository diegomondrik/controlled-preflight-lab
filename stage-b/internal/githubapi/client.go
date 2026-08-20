package githubapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBodyBytes = 4 << 20

type Client struct {
	base   *url.URL
	token  string
	client *http.Client
}

type Receipt struct {
	Method       string              `json:"method"`
	Path         string              `json:"path"`
	Status       int                 `json:"status"`
	ObservedAt   string              `json:"observed_at"`
	BodySHA256   string              `json:"body_sha256"`
	Body         json.RawMessage     `json:"body"`
	Headers      map[string][]string `json:"headers"`
	PaginationOK bool                `json:"pagination_complete"`
}

func New(rawBase, token string) (*Client, error) {
	base, err := url.Parse(rawBase)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return nil, errors.New("API_URL must be an absolute HTTPS origin")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	base.RawQuery = ""
	base.Fragment = ""
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Client{
		base:  base,
		token: token,
		client: &http.Client{
			Transport: transport,
			Timeout:   25 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Get(ctx context.Context, path string, out any) (Receipt, error) {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) Post(ctx context.Context, path string, payload, out any) (Receipt, error) {
	return c.do(ctx, http.MethodPost, path, payload, out)
}

func (c *Client) do(ctx context.Context, method, path string, payload, out any) (Receipt, error) {
	var receipt Receipt
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return receipt, errors.New("API path must be a single-origin absolute path")
	}
	requestURL := *c.base
	parsed, err := url.Parse(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return receipt, errors.New("invalid API path")
	}
	requestURL.Path = strings.TrimRight(c.base.Path, "/") + parsed.Path
	requestURL.RawQuery = parsed.RawQuery

	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return receipt, marshalErr
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return receipt, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ingol-stage-b-source-candidate")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return receipt, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return receipt, err
	}
	if len(raw) > maxBodyBytes {
		return receipt, errors.New("API response exceeded the fixed body ceiling")
	}
	digest := sha256.Sum256(raw)
	receipt = Receipt{
		Method:       method,
		Path:         path,
		Status:       resp.StatusCode,
		ObservedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		BodySHA256:   hex.EncodeToString(digest[:]),
		Body:         append(json.RawMessage(nil), raw...),
		Headers:      selectedHeaders(resp.Header),
		PaginationOK: !strings.Contains(strings.ToLower(resp.Header.Get("Link")), `rel="next"`),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return receipt, fmt.Errorf("API %s %s returned HTTP %d", method, path, resp.StatusCode)
	}
	if !receipt.PaginationOK {
		return receipt, errors.New("paginated response has an unread next page")
	}
	if out != nil {
		if len(raw) == 0 {
			return receipt, errors.New("expected JSON response body is empty")
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return receipt, fmt.Errorf("invalid JSON response: %w", err)
		}
	}
	return receipt, nil
}

func selectedHeaders(header http.Header) map[string][]string {
	wanted := []string{"Date", "Link", "X-GitHub-Request-Id", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "X-RateLimit-Resource"}
	result := make(map[string][]string, len(wanted))
	for _, key := range wanted {
		if values := header.Values(key); len(values) != 0 {
			result[key] = append([]string(nil), values...)
		}
	}
	return result
}
