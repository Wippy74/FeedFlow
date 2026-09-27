package auth

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	AudienceAPI           = "feedflow-api"
	AudienceNotifications = "feedflow-notifications"
	MaxAccessTokenTTL     = time.Hour
	accessTokenType       = "at+jwt"
)

var ErrInvalidToken = errors.New("invalid access token")

type AccessToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type Issuer struct {
	key       ed25519.PrivateKey
	keyID     string
	issuer    string
	audiences []string
	ttl       time.Duration
	now       func() time.Time
}

type Verifier struct {
	key      ed25519.PublicKey
	keyID    string
	issuer   string
	audience string
	now      func() time.Time
}

func NewIssuer(key ed25519.PrivateKey, keyID, issuer string, audiences []string, ttl time.Duration) (*Issuer, error) {
	if len(key) != ed25519.PrivateKeySize || strings.TrimSpace(keyID) == "" || strings.TrimSpace(issuer) == "" {
		return nil, fmt.Errorf("JWT issuer requires an Ed25519 private key, key ID and issuer")
	}
	if ttl < time.Second || ttl > MaxAccessTokenTTL || ttl%time.Second != 0 {
		return nil, fmt.Errorf("JWT TTL must be whole seconds between 1s and 1h")
	}
	if len(audiences) == 0 {
		return nil, fmt.Errorf("JWT issuer requires audiences")
	}
	for _, audience := range audiences {
		if strings.TrimSpace(audience) == "" {
			return nil, fmt.Errorf("JWT audience must not be empty")
		}
	}
	return &Issuer{
		key: append(ed25519.PrivateKey(nil), key...), keyID: keyID, issuer: issuer,
		audiences: append([]string(nil), audiences...), ttl: ttl, now: time.Now,
	}, nil
}

func NewVerifier(key ed25519.PublicKey, keyID, issuer, audience string) (*Verifier, error) {
	if len(key) != ed25519.PublicKeySize || strings.TrimSpace(keyID) == "" || strings.TrimSpace(issuer) == "" || strings.TrimSpace(audience) == "" {
		return nil, fmt.Errorf("JWT verifier requires an Ed25519 public key, key ID, issuer and audience")
	}
	return &Verifier{
		key: append(ed25519.PublicKey(nil), key...), keyID: keyID, issuer: issuer, audience: audience, now: time.Now,
	}, nil
}

func (i *Issuer) Issue(userID uuid.UUID) (AccessToken, error) {
	if userID == uuid.Nil {
		return AccessToken{}, fmt.Errorf("JWT subject must be a non-zero user UUID")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return AccessToken{}, fmt.Errorf("generate JWT ID: %w", err)
	}
	now := i.now().UTC().Truncate(time.Second)
	claims := jwt.RegisteredClaims{
		Issuer: i.issuer, Subject: userID.String(), Audience: jwt.ClaimStrings(i.audiences), ID: id.String(),
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["typ"] = accessTokenType
	token.Header["kid"] = i.keyID
	value, err := token.SignedString(i.key)
	if err != nil {
		return AccessToken{}, fmt.Errorf("sign access token: %w", err)
	}
	return AccessToken{AccessToken: value, TokenType: "Bearer", ExpiresIn: int64(i.ttl / time.Second)}, nil
}

func (v *Verifier) Verify(value string) (uuid.UUID, error) {
	if len(value) == 0 || len(value) > 8192 {
		return uuid.Nil, ErrInvalidToken
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(value, claims, func(token *jwt.Token) (any, error) {
		if token.Header["kid"] != v.keyID || token.Header["typ"] != accessTokenType {
			return nil, ErrInvalidToken
		}
		return v.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(),
		jwt.WithNotBeforeRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(v.now))
	if err != nil || token == nil || !token.Valid || claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.NotBefore == nil {
		return uuid.Nil, ErrInvalidToken
	}
	lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > MaxAccessTokenTTL || claims.NotBefore.After(claims.ExpiresAt.Time) {
		return uuid.Nil, ErrInvalidToken
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil || userID == uuid.Nil {
		return uuid.Nil, ErrInvalidToken
	}
	if id, err := uuid.Parse(claims.ID); err != nil || id == uuid.Nil {
		return uuid.Nil, ErrInvalidToken
	}
	return userID, nil
}

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JWT private key: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, fmt.Errorf("JWT private key must be a single PKCS#8 PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse JWT private key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("JWT private key must use Ed25519")
	}
	return key, nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JWT public key: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, fmt.Errorf("JWT public key must be a single PKIX PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse JWT public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("JWT public key must use Ed25519")
	}
	return key, nil
}
