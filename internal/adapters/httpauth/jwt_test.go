package httpauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestJWTVerifierAcceptsAllowedSupabaseToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := jwkSet{Keys: []jwk{{
		Kty: "RSA",
		Kid: "test-key",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	httpClient := jwksClient(t, jwks)

	verifier, err := NewJWTVerifier(VerifierConfig{
		IssuerURL:    "https://project.supabase.co/auth/v1",
		Audience:     "authenticated",
		AllowedEmail: "me@example.com",
		JWKSURL:      "https://project.supabase.co/auth/v1/.well-known/jwks.json",
		HTTPClient:   httpClient,
	})
	if err != nil {
		t.Fatal(err)
	}

	token := signTestJWT(t, key, map[string]any{
		"iss":   "https://project.supabase.co/auth/v1",
		"sub":   "user-1",
		"email": "me@example.com",
		"aud":   "authenticated",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"scope": "openid intervals.read",
	})

	info, err := verifier.Verify(context.Background(), token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Expiration.IsZero() {
		t.Fatal("missing expiration")
	}
}

func TestJWTVerifierRejectsWrongEmail(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := jwkSet{Keys: []jwk{{
		Kty: "RSA",
		Kid: "test-key",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	httpClient := jwksClient(t, jwks)

	verifier, err := NewJWTVerifier(VerifierConfig{
		IssuerURL:    "https://project.supabase.co/auth/v1",
		Audience:     "authenticated",
		AllowedEmail: "me@example.com",
		JWKSURL:      "https://project.supabase.co/auth/v1/.well-known/jwks.json",
		HTTPClient:   httpClient,
	})
	if err != nil {
		t.Fatal(err)
	}

	token := signTestJWT(t, key, map[string]any{
		"iss":   "https://project.supabase.co/auth/v1",
		"sub":   "user-1",
		"email": "other@example.com",
		"aud":   "authenticated",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	if _, err := verifier.Verify(context.Background(), token, nil); err == nil {
		t.Fatal("Verify() succeeded for wrong email")
	}
}

func signTestJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": "test-key", "typ": "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := []byte(encodedHeader + "." + encodedClaims)
	digest := sha256.Sum256(signingInput)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return string(signingInput) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func jwksClient(t *testing.T, set jwkSet) *http.Client {
	t.Helper()
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
