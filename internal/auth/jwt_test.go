package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return publicKey, privateKey
}

func TestAccessTokenCanBeVerifiedByIndependentServices(t *testing.T) {
	publicKey, privateKey := testKeys(t)
	issuer, err := NewIssuer(privateKey, "key-1", "feedflow", []string{AudienceAPI, AudienceNotifications}, 15*time.Minute)
	require.NoError(t, err)
	userID := uuid.New()
	token, err := issuer.Issue(userID)
	require.NoError(t, err)
	assert.Equal(t, "Bearer", token.TokenType)
	assert.Equal(t, int64(900), token.ExpiresIn)
	for _, audience := range []string{AudienceAPI, AudienceNotifications} {
		verifier, err := NewVerifier(publicKey, "key-1", "feedflow", audience)
		require.NoError(t, err)
		got, err := verifier.Verify(token.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, userID, got)
	}
	secondToken, err := issuer.Issue(userID)
	require.NoError(t, err)
	assert.NotEqual(t, token.AccessToken, secondToken.AccessToken, "JWT IDs must be unique")
	_, err = issuer.Issue(uuid.Nil)
	require.Error(t, err)
}

func TestVerifierRejectsInvalidTokens(t *testing.T) {
	publicKey, privateKey := testKeys(t)
	_, wrongKey := testKeys(t)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	verifier, err := NewVerifier(publicKey, "key-1", "feedflow", AudienceAPI)
	require.NoError(t, err)
	verifier.now = func() time.Time { return now }

	tests := []struct {
		name   string
		mutate func(*jwt.RegisteredClaims, *jwt.Token)
		key    any
	}{
		{name: "expired", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second)) }},
		{name: "expiration boundary", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.ExpiresAt = jwt.NewNumericDate(now) }},
		{name: "missing expiry", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.ExpiresAt = nil }},
		{name: "missing nbf", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.NotBefore = nil }},
		{name: "future nbf", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.NotBefore = jwt.NewNumericDate(now.Add(time.Minute)) }},
		{name: "missing iat", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.IssuedAt = nil }},
		{name: "future iat", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute)) }},
		{name: "wrong issuer", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Issuer = "other" }},
		{name: "missing issuer", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Issuer = "" }},
		{name: "wrong audience", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Audience = jwt.ClaimStrings{"other"} }},
		{name: "missing audience", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Audience = nil }},
		{name: "invalid subject", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Subject = "not-a-uuid" }},
		{name: "zero subject", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.Subject = uuid.Nil.String() }},
		{name: "missing ID", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.ID = "" }},
		{name: "too long lifetime", mutate: func(c *jwt.RegisteredClaims, _ *jwt.Token) { c.ExpiresAt = jwt.NewNumericDate(now.Add(2 * time.Hour)) }},
		{name: "wrong key ID", mutate: func(_ *jwt.RegisteredClaims, token *jwt.Token) { token.Header["kid"] = "other" }},
		{name: "wrong token type", mutate: func(_ *jwt.RegisteredClaims, token *jwt.Token) { token.Header["typ"] = "JWT" }},
		{name: "wrong signing key", key: wrongKey},
		{name: "HMAC algorithm", key: []byte("secret"), mutate: func(_ *jwt.RegisteredClaims, token *jwt.Token) { token.Method = jwt.SigningMethodHS256 }},
		{name: "none algorithm", key: jwt.UnsafeAllowNoneSignatureType, mutate: func(_ *jwt.RegisteredClaims, token *jwt.Token) { token.Method = jwt.SigningMethodNone }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &jwt.RegisteredClaims{
				Issuer: "feedflow", Subject: uuid.NewString(), Audience: jwt.ClaimStrings{AudienceAPI}, ID: uuid.NewString(),
				IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
				ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			}
			token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
			token.Header["kid"] = "key-1"
			token.Header["typ"] = accessTokenType
			if tt.mutate != nil {
				tt.mutate(claims, token)
			}
			key := tt.key
			if key == nil {
				key = privateKey
			}
			value, err := token.SignedString(key)
			require.NoError(t, err)
			got, err := verifier.Verify(value)
			require.ErrorIs(t, err, ErrInvalidToken)
			assert.Equal(t, uuid.Nil, got)
		})
	}
	for _, value := range []string{"", "malformed", strings.Repeat("a", 8193)} {
		_, err := verifier.Verify(value)
		require.ErrorIs(t, err, ErrInvalidToken)
	}
}

func TestJWTKeyFiles(t *testing.T) {
	publicKey, privateKey := testKeys(t)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	privatePath := filepath.Join(t.TempDir(), "private.pem")
	publicPath := filepath.Join(t.TempDir(), "public.pem")
	require.NoError(t, os.WriteFile(privatePath, privatePEM, 0600))
	require.NoError(t, os.WriteFile(publicPath, publicPEM, 0600))
	gotPrivate, err := LoadPrivateKey(privatePath)
	require.NoError(t, err)
	assert.Equal(t, privateKey, gotPrivate)
	gotPublic, err := LoadPublicKey(publicPath)
	require.NoError(t, err)
	assert.Equal(t, publicKey, gotPublic)
	_, err = LoadPrivateKey(publicPath)
	require.Error(t, err)
	_, err = LoadPublicKey(privatePath)
	require.Error(t, err)
	_, err = LoadPrivateKey(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	_, err = LoadPublicKey(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	for _, value := range [][]byte{[]byte("invalid"), append(publicPEM, publicPEM...), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("invalid")})} {
		require.NoError(t, os.WriteFile(publicPath, value, 0600))
		_, err := LoadPublicKey(publicPath)
		require.Error(t, err)
	}
	for _, value := range [][]byte{[]byte("invalid"), append(privatePEM, privatePEM...), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("invalid")})} {
		require.NoError(t, os.WriteFile(privatePath, value, 0600))
		_, err := LoadPrivateKey(privatePath)
		require.Error(t, err)
	}
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	otherPrivateDER, err := x509.MarshalPKCS8PrivateKey(otherKey)
	require.NoError(t, err)
	otherPublicDER, err := x509.MarshalPKIXPublicKey(&otherKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: otherPrivateDER}), 0600))
	require.NoError(t, os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherPublicDER}), 0600))
	_, err = LoadPrivateKey(privatePath)
	require.ErrorContains(t, err, "Ed25519")
	_, err = LoadPublicKey(publicPath)
	require.ErrorContains(t, err, "Ed25519")
}

func TestDevelopmentKeyGenerator(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not installed")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	keyDir := t.TempDir()
	script := filepath.Join("..", "..", "deploy", "auth", "generate-dev-keys.sh")
	output, err := exec.Command("sh", script, keyDir).CombinedOutput()
	require.NoError(t, err, string(output))
	privatePath := filepath.Join(keyDir, "private.pem")
	publicPath := filepath.Join(keyDir, "public.pem")
	privateKey, err := LoadPrivateKey(privatePath)
	require.NoError(t, err)
	publicKey, err := LoadPublicKey(publicPath)
	require.NoError(t, err)
	assert.True(t, privateKey.Public().(ed25519.PublicKey).Equal(publicKey))
	info, err := os.Stat(privatePath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0077, "private key must not be readable by group or others")
	before, err := os.ReadFile(privatePath)
	require.NoError(t, err)
	output, err = exec.Command("sh", script, keyDir).CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "Refusing to overwrite")
	after, err := os.ReadFile(privatePath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestJWTConstructorsRejectInvalidConfig(t *testing.T) {
	publicKey, privateKey := testKeys(t)
	for _, ttl := range []time.Duration{0, -time.Second, time.Millisecond, time.Second + time.Millisecond, 2 * time.Hour} {
		_, err := NewIssuer(privateKey, "key", "issuer", []string{AudienceAPI}, ttl)
		require.Error(t, err)
	}
	_, err := NewIssuer(nil, "key", "issuer", []string{AudienceAPI}, time.Minute)
	require.Error(t, err)
	_, err = NewIssuer(privateKey, "", "issuer", []string{AudienceAPI}, time.Minute)
	require.Error(t, err)
	_, err = NewIssuer(privateKey, "key", "", []string{AudienceAPI}, time.Minute)
	require.Error(t, err)
	_, err = NewIssuer(privateKey, "key", "issuer", nil, time.Minute)
	require.Error(t, err)
	_, err = NewIssuer(privateKey, "key", "issuer", []string{" "}, time.Minute)
	require.Error(t, err)
	_, err = NewVerifier(nil, "key", "issuer", AudienceAPI)
	require.Error(t, err)
	_, err = NewVerifier(publicKey, "key", "issuer", "")
	require.Error(t, err)
}
