package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testSecret = "12345678901234567890123456789012"

func TestTokenManager_GenerateAndParse(t *testing.T) {
	t.Parallel()

	tm, err := NewTokenManager(testSecret, time.Hour)
	require.NoError(t, err)

	fixedNow := time.Unix(1_700_000_000, 0)
	tm.now = func() time.Time { return fixedNow }

	token, err := tm.GenerateToken(42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := tm.ParseToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(42), userID)
}

func TestTokenManager_ParseToken_Expired(t *testing.T) {
	t.Parallel()

	tm, err := NewTokenManager(testSecret, time.Minute)
	require.NoError(t, err)

	issuedAt := time.Unix(1_700_000_000, 0)
	tm.now = func() time.Time { return issuedAt }

	token, err := tm.GenerateToken(42)
	require.NoError(t, err)

	tm.now = func() time.Time { return issuedAt.Add(time.Minute) } // tm.now().Unix() == exp

	_, err = tm.ParseToken(token)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrExpiredToken))
}

func TestTokenManager_ParseToken_BadSignature(t *testing.T) {
	t.Parallel()

	tm, err := NewTokenManager(testSecret, time.Hour)
	require.NoError(t, err)

	token, err := tm.GenerateToken(42)
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 2)

	sigRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	sigRaw[0] ^= 0xFF
	parts[1] = base64.RawURLEncoding.EncodeToString(sigRaw)
	tamperedToken := parts[0] + "." + parts[1]

	_, err = tm.ParseToken(tamperedToken)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidToken))
}

func TestTokenManager_ParseToken_Malformed(t *testing.T) {
	t.Parallel()

	tm, err := NewTokenManager(testSecret, time.Hour)
	require.NoError(t, err)

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "missing dot",
			token: "abc",
		},
		{
			name:  "bad signature base64",
			token: "payload.!@#$",
		},
		{
			name:  "payload is not base64",
			token: "!@#." + signPayload("!@#", []byte(testSecret)),
		},
		{
			name:  "payload is not json",
			token: makeSignedToken("not-json", []byte(testSecret)),
		},
		{
			name:  "payload has invalid user id",
			token: makeSignedToken(`{"uid":0,"exp":4102444800,"iat":1700000000}`, []byte(testSecret)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tm.ParseToken(tt.token)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrInvalidToken))
		})
	}
}

func makeSignedToken(payloadJSON string, secret []byte) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return payload + "." + signPayload(payload, secret)
}

func signPayload(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
