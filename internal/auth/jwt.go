package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

const jwksCacheTTL = 10 * time.Minute

type JWTVerifier struct {
	issuer       string
	audience     string
	allowedEmail string
	allowedSub   string
	jwksURL      string
	client       *http.Client

	mu        sync.Mutex
	cachedSet jwkSet
	cachedAt  time.Time
}

type VerifierConfig struct {
	IssuerURL      string
	Audience       string
	AllowedEmail   string
	AllowedSubject string
	JWKSURL        string
	HTTPClient     *http.Client
}

func NewJWTVerifier(cfg VerifierConfig) (*JWTVerifier, error) {
	if cfg.IssuerURL == "" {
		return nil, errors.New("issuer URL is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("audience is required")
	}
	if cfg.JWKSURL == "" {
		return nil, errors.New("JWKS URL is required")
	}
	if cfg.AllowedEmail == "" && cfg.AllowedSubject == "" {
		return nil, errors.New("allowed email or subject is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &JWTVerifier{
		issuer:       strings.TrimRight(cfg.IssuerURL, "/"),
		audience:     cfg.Audience,
		allowedEmail: strings.ToLower(strings.TrimSpace(cfg.AllowedEmail)),
		allowedSub:   strings.TrimSpace(cfg.AllowedSubject),
		jwksURL:      cfg.JWKSURL,
		client:       client,
	}, nil
}

func (v *JWTVerifier) Verify(ctx context.Context, token string, req *http.Request) (*mcpauth.TokenInfo, error) {
	_ = req

	parsed, err := parseJWT(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
	}

	if err := v.verifySignature(ctx, parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
	}

	var claims tokenClaims
	if err := json.Unmarshal(parsed.payload, &claims); err != nil {
		return nil, fmt.Errorf("%w: invalid claims: %v", mcpauth.ErrInvalidToken, err)
	}
	if err := v.validateClaims(claims, time.Now()); err != nil {
		return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
	}

	return &mcpauth.TokenInfo{
		Scopes:     claims.scopes(),
		Expiration: time.Unix(claims.Exp, 0),
		Extra: map[string]any{
			"iss":   claims.Iss,
			"sub":   claims.Sub,
			"email": claims.Email,
			"aud":   claims.Aud,
		},
	}, nil
}

func (v *JWTVerifier) validateClaims(claims tokenClaims, now time.Time) error {
	if strings.TrimRight(claims.Iss, "/") != v.issuer {
		return fmt.Errorf("issuer %q does not match expected issuer", claims.Iss)
	}
	if claims.Sub == "" {
		return errors.New("subject is missing")
	}
	if claims.Exp == 0 {
		return errors.New("expiration is missing")
	}
	if now.After(time.Unix(claims.Exp, 0)) {
		return errors.New("token is expired")
	}
	if claims.Nbf != 0 && now.Before(time.Unix(claims.Nbf, 0)) {
		return errors.New("token is not valid yet")
	}
	if !claims.hasAudience(v.audience) {
		return fmt.Errorf("audience %q is missing", v.audience)
	}
	if v.allowedSub != "" && claims.Sub == v.allowedSub {
		return nil
	}
	if v.allowedEmail != "" && strings.EqualFold(claims.Email, v.allowedEmail) {
		return nil
	}
	return errors.New("token subject/email is not allowed")
}

func (v *JWTVerifier) verifySignature(ctx context.Context, token parsedJWT) error {
	if token.header.Alg == "" || token.header.Alg == "none" {
		return errors.New("unsupported signing algorithm")
	}
	set, err := v.getJWKSet(ctx, false)
	if err != nil {
		return err
	}
	key, ok := set.find(token.header.Kid, token.header.Alg)
	if !ok {
		set, err = v.getJWKSet(ctx, true)
		if err != nil {
			return err
		}
		key, ok = set.find(token.header.Kid, token.header.Alg)
		if !ok {
			return errors.New("matching JWK not found")
		}
	}
	return verifyJWKSignature(key, token.header.Alg, token.signingInput, token.signature)
}

func (v *JWTVerifier) getJWKSet(ctx context.Context, refresh bool) (jwkSet, error) {
	v.mu.Lock()
	if !refresh && len(v.cachedSet.Keys) > 0 && time.Since(v.cachedAt) < jwksCacheTTL {
		set := v.cachedSet
		v.mu.Unlock()
		return set, nil
	}
	v.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return jwkSet{}, fmt.Errorf("build JWKS request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := v.client.Do(req)
	if err != nil {
		return jwkSet{}, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return jwkSet{}, fmt.Errorf("fetch JWKS: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var set jwkSet
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&set); err != nil {
		return jwkSet{}, fmt.Errorf("decode JWKS: %w", err)
	}
	if len(set.Keys) == 0 {
		return jwkSet{}, errors.New("JWKS contains no keys")
	}

	v.mu.Lock()
	v.cachedSet = set
	v.cachedAt = time.Now()
	v.mu.Unlock()
	return set, nil
}

type parsedJWT struct {
	header       jwtHeader
	payload      []byte
	signature    []byte
	signingInput []byte
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type tokenClaims struct {
	Iss    string   `json:"iss"`
	Sub    string   `json:"sub"`
	Email  string   `json:"email"`
	Aud    audience `json:"aud"`
	Exp    int64    `json:"exp"`
	Nbf    int64    `json:"nbf"`
	Iat    int64    `json:"iat"`
	Scope  string   `json:"scope"`
	Scp    []string `json:"scp"`
	Scopes []string `json:"scopes"`
}

func (c tokenClaims) hasAudience(want string) bool {
	return slices.Contains(c.Aud.Values, want)
}

func (c tokenClaims) scopes() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	for scope := range strings.FieldsSeq(c.Scope) {
		add(scope)
	}
	for _, scope := range c.Scp {
		add(scope)
	}
	for _, scope := range c.Scopes {
		add(scope)
	}
	return out
}

type audience struct {
	Values []string
}

func (a *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		a.Values = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	a.Values = many
	return nil
}

func parseJWT(token string) (parsedJWT, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return parsedJWT{}, errors.New("token must have three segments")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return parsedJWT{}, fmt.Errorf("decode header: %w", err)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return parsedJWT{}, fmt.Errorf("decode payload: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return parsedJWT{}, fmt.Errorf("decode signature: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return parsedJWT{}, fmt.Errorf("decode header JSON: %w", err)
	}
	return parsedJWT{
		header:       header,
		payload:      payloadBytes,
		signature:    signature,
		signingInput: []byte(parts[0] + "." + parts[1]),
	}, nil
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

func (s jwkSet) find(kid, alg string) (jwk, bool) {
	for _, key := range s.Keys {
		if kid != "" && key.Kid != "" && kid != key.Kid {
			continue
		}
		if key.Alg != "" && key.Alg != alg {
			continue
		}
		return key, true
	}
	return jwk{}, false
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func verifyJWKSignature(key jwk, alg string, signingInput, signature []byte) error {
	switch alg {
	case "RS256", "RS384", "RS512":
		pub, err := key.rsaPublicKey()
		if err != nil {
			return err
		}
		hash, digest, err := digestFor(alg, signingInput)
		if err != nil {
			return err
		}
		return rsa.VerifyPKCS1v15(pub, hash, digest, signature)
	case "ES256", "ES384", "ES512":
		pub, size, err := key.ecdsaPublicKey()
		if err != nil {
			return err
		}
		if len(signature) != size*2 {
			return fmt.Errorf("ECDSA signature length %d does not match curve size", len(signature))
		}
		_, digest, err := digestFor(alg, signingInput)
		if err != nil {
			return err
		}
		r := new(big.Int).SetBytes(signature[:size])
		s := new(big.Int).SetBytes(signature[size:])
		if !ecdsa.Verify(pub, digest, r, s) {
			return errors.New("ECDSA signature verification failed")
		}
		return nil
	case "EdDSA":
		pub, err := key.ed25519PublicKey()
		if err != nil {
			return err
		}
		if !ed25519.Verify(pub, signingInput, signature) {
			return errors.New("EdDSA signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported signing algorithm %q", alg)
	}
}

func digestFor(alg string, signingInput []byte) (crypto.Hash, []byte, error) {
	switch alg {
	case "RS256", "ES256":
		sum := sha256.Sum256(signingInput)
		return crypto.SHA256, sum[:], nil
	case "RS384", "ES384":
		sum := sha512.Sum384(signingInput)
		return crypto.SHA384, sum[:], nil
	case "RS512", "ES512":
		sum := sha512.Sum512(signingInput)
		return crypto.SHA512, sum[:], nil
	default:
		return 0, nil, fmt.Errorf("unsupported signing algorithm %q", alg)
	}
}

func (k jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, fmt.Errorf("JWK kty %q is not RSA", k.Kty)
	}
	nBytes, err := decodeJWKInt(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode RSA modulus: %w", err)
	}
	eBytes, err := decodeJWKInt(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode RSA exponent: %w", err)
	}
	e := new(big.Int).SetBytes(eBytes).Int64()
	if e <= 1 || e > 1<<31-1 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e)}, nil
}

func (k jwk) ecdsaPublicKey() (*ecdsa.PublicKey, int, error) {
	if k.Kty != "EC" {
		return nil, 0, fmt.Errorf("JWK kty %q is not EC", k.Kty)
	}
	var curve elliptic.Curve
	var size int
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
		size = 32
	case "P-384":
		curve = elliptic.P384()
		size = 48
	case "P-521":
		curve = elliptic.P521()
		size = 66
	default:
		return nil, 0, fmt.Errorf("unsupported EC curve %q", k.Crv)
	}
	xBytes, err := decodeJWKInt(k.X)
	if err != nil {
		return nil, 0, fmt.Errorf("decode EC x: %w", err)
	}
	yBytes, err := decodeJWKInt(k.Y)
	if err != nil {
		return nil, 0, fmt.Errorf("decode EC y: %w", err)
	}
	return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}, size, nil
}

func (k jwk) ed25519PublicKey() (ed25519.PublicKey, error) {
	if k.Kty != "OKP" || k.Crv != "Ed25519" {
		return nil, fmt.Errorf("JWK kty/crv %q/%q is not Ed25519", k.Kty, k.Crv)
	}
	x, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("decode OKP x: %w", err)
	}
	if len(x) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key size")
	}
	return ed25519.PublicKey(x), nil
}

func decodeJWKInt(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("empty value")
	}
	return base64.RawURLEncoding.DecodeString(value)
}
