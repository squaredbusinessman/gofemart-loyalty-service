package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type tokenPayload struct {
	UserID int64 `json:"uid"`
	Exp    int64 `json:"exp"`
	Iat    int64 `json:"iat"`
}

type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("secret too short: need >= 32 bytes")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("ttl must be > 0")
	}
	return &TokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}, nil
}

func (tm *TokenManager) GenerateToken(userID int64) (string, error) {
	now := tm.now()

	p := tokenPayload{
		UserID: userID,
		Iat:    now.Unix(),
		Exp:    now.Add(tm.ttl).Unix(),
	}

	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, tm.secret)
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payload + "." + sig, nil
}

func (tm *TokenManager) ParseToken(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return 0, ErrInvalidToken
	}

	payload, sig := parts[0], parts[1]

	mac := hmac.New(sha256.New, tm.secret)
	_, _ = mac.Write([]byte(payload))
	expextedSig := mac.Sum(nil)

	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, expextedSig) {
		return 0, ErrInvalidToken
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return 0, ErrInvalidToken
	}

	var p tokenPayload
	err = json.Unmarshal(raw, &p)
	if err != nil || p.UserID <= 0 {
		return 0, ErrInvalidToken
	}
	if tm.now().Unix() >= p.Exp {
		return 0, ErrExpiredToken
	}
	return p.UserID, nil
}
