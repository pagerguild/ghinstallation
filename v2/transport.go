package ghinstallation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v69/github"
)

const (
	acceptHeader = "application/vnd.github.v3+json"
	apiBaseURL   = "https://api.github.com"
)

// Transport provides a http.RoundTripper by wrapping an existing
// http.RoundTripper and provides GitHub Apps authentication as an
// installation.
//
// Client can also be overwritten, and is useful to change to one which
// provides retry logic if you do experience retryable errors.
//
// See https://developer.github.com/apps/building-integrations/setting-up-and-registering-github-apps/about-authentication-options-for-github-apps/
type Transport struct {
	BaseURL                  string                           // BaseURL is the scheme and host for GitHub API, defaults to https://api.github.com
	Client                   Client                           // Client to use to refresh tokens, defaults to http.Client with provided transport
	tr                       http.RoundTripper                // tr is the underlying roundtripper being wrapped
	appID                    string                           // appID is the GitHub App's ID
	installationID           int64                            // installationID is the GitHub App Installation ID
	InstallationTokenOptions *github.InstallationTokenOptions // parameters restrict a token's access
	appsTransport            *AppsTransport

	mu    sync.Mutex   // mu protects token
	token *accessToken // token is the installation's access token
}

// accessToken is an installation access token response from GitHub
type accessToken struct {
	Token        string                         `json:"token"`
	ExpiresAt    time.Time                      `json:"expires_at"`
	Permissions  github.InstallationPermissions `json:"permissions,omitempty"`
	Repositories []github.Repository            `json:"repositories,omitempty"`
}

// HTTPError represents a custom error for failing HTTP operations.
// Example in our usecase: refresh access token operation.
// It enables the caller to inspect the root cause and response.
type HTTPError struct {
	Message        string
	RootCause      error
	InstallationID int64
	Response       *http.Response
}

func (e *HTTPError) Error() string {
	return e.Message
}

var _ http.RoundTripper = &Transport{}

// NewKeyFromFile returns a Transport using a private key from file.
func NewKeyFromFile(tr http.RoundTripper, appID string, installationID int64, privateKeyFile string) (*Transport, error) {
	privateKey, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("could not read private key: %s", err)
	}
	return New(tr, appID, installationID, privateKey)
}

// Client is a HTTP client which sends a http.Request and returns a http.Response
// or an error.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// New returns an Transport using private key. The key is parsed
// and if any errors occur the error is non-nil.
//
// The provided tr http.RoundTripper should be shared between multiple
// installations to ensure reuse of underlying TCP connections.
//
// The returned Transport's RoundTrip method is safe to be used concurrently.
func New(tr http.RoundTripper, appID string, installationID int64, privateKey []byte) (*Transport, error) {
	atr, err := NewAppsTransport(tr, appID, privateKey)
	if err != nil {
		return nil, err
	}

	return NewFromAppsTransport(atr, installationID), nil
}

// NewFromAppsTransport returns a Transport using an existing *AppsTransport.
func NewFromAppsTransport(atr *AppsTransport, installationID int64) *Transport {
	return &Transport{
		BaseURL:        atr.BaseURL,
		Client:         &http.Client{Transport: atr.tr},
		tr:             atr.tr,
		appID:          atr.clientID,
		installationID: installationID,
		appsTransport:  atr,
	}
}

// RoundTrip implements http.RoundTripper interface.
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {

	token, err := t.Token(req.Context())
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Add("Accept", acceptHeader) // We add to "Accept" header to avoid overwriting existing req headers.
	resp, err = t.tr.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

// Token checks the active token expiration and renews if necessary. Token returns
// a valid access token. If renewal fails an error is returned.
func (t *Transport) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == nil || t.token.ExpiresAt.Add(-time.Minute).Before(time.Now()) {
		// Token is not set or expired/nearly expired, so refresh
		if err := t.refreshToken(ctx); err != nil {
			return "", fmt.Errorf("could not refresh installation id %v's token: %w", t.installationID, err)
		}
	}

	return t.token.Token, nil
}

// Permissions returns a transport token's GitHub installation permissions.
func (t *Transport) Permissions() (github.InstallationPermissions, error) {
	if t.token == nil {
		return github.InstallationPermissions{}, fmt.Errorf("Permissions() = nil, err: nil token")
	}
	return t.token.Permissions, nil
}

// Repositories returns a transport token's GitHub repositories.
func (t *Transport) Repositories() ([]github.Repository, error) {
	if t.token == nil {
		return nil, fmt.Errorf("Repositories() = nil, err: nil token")
	}
	return t.token.Repositories, nil
}

func (t *Transport) refreshToken(ctx context.Context) error {
	// Convert InstallationTokenOptions into a ReadWriter to pass as an argument to http.NewRequest.
	body, err := GetReadWriter(t.InstallationTokenOptions)
	if err != nil {
		return fmt.Errorf("could not convert installation token parameters into json: %s", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/app/installations/%v/access_tokens", t.BaseURL, t.installationID), body)
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	// Set Content and Accept headers.
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", acceptHeader)

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	t.appsTransport.BaseURL = t.BaseURL
	t.appsTransport.Client = t.Client
	var resp *http.Response

	resp, err = t.appsTransport.RoundTrip(req)
	e := &HTTPError{
		RootCause:      err,
		InstallationID: t.installationID,
		Response:       resp,
	}
	if err != nil {
		e.Message = fmt.Sprintf("could not get access_tokens from GitHub API for installation ID %v: %v", t.installationID, err)
		return e
	}

	if resp.StatusCode/100 != 2 {
		e.Message = fmt.Sprintf("received non 2xx response status %q when fetching %v", resp.Status, req.URL)
		return e
	}
	// Closing body late, to provide caller a chance to inspect body in an error / non-200 response status situation
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(&t.token)
}

// GetReadWriter converts a body interface into an io.ReadWriter object.
func GetReadWriter(i interface{}) (io.ReadWriter, error) {
	var buf io.ReadWriter
	if i != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := enc.Encode(i)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}
