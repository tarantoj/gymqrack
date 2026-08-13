// Package vivagym is a stateless proxy client for the VivaGym API.
//
// The server holds no user state: every request is an independent OAuth2
// call, and VivaGym tokens are owned by the browser (see internal/server).
package vivagym

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const BaseURL = "https://vivagym.myvitale.com"

const appName = "vivagym"

// VivaGymError carries the HTTP status of a failed upstream call.
type VivaGymError struct {
	Message string
	Status  int
}

func (e *VivaGymError) Error() string { return e.Message }

// TokenPair is the member's VivaGym token pair.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	// AccessToken lifetime in seconds.
	ExpiresIn int
}

// Client makes authenticated calls to the VivaGym API.
type Client struct {
	BaseURL string
	// ClientID / ClientSecret are the app-level credentials used for the
	// anonymous client_credentials grant (stage 1 of login).
	ClientID     string
	ClientSecret string
	// Locale for the login endpoint (es, en, pt).
	Locale string
	HTTP   *http.Client
}

// request performs an HTTP request and decodes a JSON (or raw text) response,
// raising VivaGymError on non-2xx responses.
func (c *Client) request(ctx context.Context, method, endpoint string, header http.Header, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, &VivaGymError{Message: upstreamMessage(data, res.StatusCode), Status: res.StatusCode}
	}
	return data, nil
}

func upstreamMessage(data []byte, status int) string {
	var parsed struct {
		Message          string `json:"message"`
		ErrorDescription string `json:"error_description"`
	}
	if json.Unmarshal(data, &parsed) == nil {
		if parsed.Message != "" {
			return parsed.Message
		}
		if parsed.ErrorDescription != "" {
			return parsed.ErrorDescription
		}
	}
	return fmt.Sprintf("VivaGym API %d", status)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Stage 1: anonymous app-level token (client_credentials grant).
func (c *Client) clientCredentials(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
	})
	data, err := c.request(ctx, http.MethodPost, "/oauth/v2/token",
		http.Header{"Content-Type": {"application/json"}}, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	var resp tokenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", &VivaGymError{Message: "client_credentials returned an invalid response", Status: 502}
	}
	if resp.AccessToken == "" {
		return "", &VivaGymError{Message: "client_credentials returned no access_token", Status: 502}
	}
	return resp.AccessToken, nil
}

// Stage 2: member login with email + password -> token pair.
func (c *Client) Login(ctx context.Context, email, password string) (TokenPair, error) {
	tempToken, err := c.clientCredentials(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	form := url.Values{}
	form.Set("access_token", tempToken)
	form.Set("email", email)
	form.Set("password", password)
	form.Set("appName", appName)
	data, err := c.request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2.0/%s/exerp/newAuth", c.Locale),
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		strings.NewReader(form.Encode()))
	if err != nil {
		return TokenPair{}, err
	}
	var resp tokenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return TokenPair{}, &VivaGymError{Message: "login returned an invalid response", Status: 502}
	}
	if resp.AccessToken == "" {
		return TokenPair{}, &VivaGymError{Message: "login returned no access_token", Status: 502}
	}
	if resp.ExpiresIn == 0 {
		resp.ExpiresIn = 600
	}
	return TokenPair{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	}, nil
}

// RefreshTokens renews the access token using the refresh token.
func (c *Client) RefreshTokens(ctx context.Context, refreshToken string) (TokenPair, error) {
	endpoint := "/api/email/refresh?refresh_token=" + url.QueryEscape(refreshToken)
	data, err := c.request(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return TokenPair{}, err
	}
	var resp tokenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return TokenPair{}, &VivaGymError{Message: "refresh returned an invalid response", Status: 502}
	}
	if resp.AccessToken == "" {
		return TokenPair{}, &VivaGymError{Message: "refresh returned no access_token", Status: 502}
	}
	if resp.ExpiresIn == 0 {
		resp.ExpiresIn = 600
	}
	if resp.RefreshToken == "" {
		resp.RefreshToken = refreshToken
	}
	return TokenPair{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	}, nil
}

// FetchQr returns the gym-entry QR payload for the given access token. The API
// returns a JSON-encoded string, e.g. "exerp:checkin:...".
func (c *Client) FetchQr(ctx context.Context, accessToken string) (string, error) {
	data, err := c.request(ctx, http.MethodGet, "/api/v2.0/exerp/qr",
		http.Header{"Authorization": {"Bearer " + accessToken}}, nil)
	if err != nil {
		return "", err
	}
	var payload string
	if json.Unmarshal(data, &payload) == nil {
		return payload, nil
	}
	return string(data), nil
}

// New returns a client with sensible defaults. baseURL may be empty.
func New(baseURL, clientID, clientSecret, locale string) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	if locale == "" {
		locale = "es"
	}
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Locale:       locale,
		HTTP: &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   30 * time.Second,
		},
	}
}
