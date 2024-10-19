package ghinstallation

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
)

func TestNewAppsTransportKeyFromFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write(key); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = NewAppsTransportKeyFromFile(&http.Transport{}, appID, tmpfile.Name())
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
}

type RoundTrip struct {
	rt func(*http.Request) (*http.Response, error)
}

func (r RoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.rt(req)
}

func TestAppsTransport(t *testing.T) {
	customHeader := "my-header"
	check := RoundTrip{
		rt: func(req *http.Request) (*http.Response, error) {
			h, ok := req.Header["Accept"]
			if !ok {
				t.Error("Header Accept not set")
			}
			want := []string{customHeader, acceptHeader}
			if diff := cmp.Diff(want, h); diff != "" {
				t.Errorf("HTTP Accept headers want->got: %s", diff)
			}
			return nil, nil
		},
	}

	tr, err := NewAppsTransport(check, appID, key)
	if err != nil {
		t.Fatalf("error creating transport: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com", new(bytes.Buffer))
	req.Header.Add("Accept", customHeader)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("error calling RoundTrip: %v", err)
	}
}

func TestJWTExpiry(t *testing.T) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(key)
	if err != nil {
		t.Fatal(err)
	}

	customHeader := "my-header"
	check := RoundTrip{
		rt: func(req *http.Request) (*http.Response, error) {
			token := strings.Fields(req.Header.Get("Authorization"))[1]

			// Use jwt.NewParser and the custom keyfunc
			tok, err := jwt.NewParser().ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
				return key.Public(), nil
			})
			if err != nil {
				t.Fatalf("jwt parse: %v", err)
			}

			c := tok.Claims.(*jwt.RegisteredClaims)
			if c.ExpiresAt == nil {
				t.Fatalf("missing exp claim")
			} else if c.ExpiresAt.Time != c.ExpiresAt.Truncate(time.Second) {
				t.Fatalf("bad exp %v: not truncated to whole seconds", c.ExpiresAt.Time)
			}
			return nil, nil
		},
	}

	tr := NewAppsTransportFromPrivateKey(check, appID, key)
	req := httptest.NewRequest(http.MethodGet, "http://example.com", new(bytes.Buffer))
	req.Header.Add("Accept", customHeader)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("error calling RoundTrip: %v", err)
	}
}
