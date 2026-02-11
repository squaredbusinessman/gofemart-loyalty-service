package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_ProducesValidBcryptHash(t *testing.T) {
	t.Parallel()

	password := "strong-password-123"

	hash, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.NotEqual(t, password, hash)

	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, bcrypt.DefaultCost, cost)

	require.NoError(t, CheckPassword(hash, password))
}

func TestHashPassword_SamePasswordDifferentHashes(t *testing.T) {
	t.Parallel()

	password := "same-password"

	hash1, err := HashPassword(password)
	require.NoError(t, err)

	hash2, err := HashPassword(password)
	require.NoError(t, err)

	require.NotEqual(t, hash1, hash2)
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("")
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	require.NoError(t, CheckPassword(hash, ""))
}

func TestHashPassword_PasswordTooLong(t *testing.T) {
	t.Parallel()

	tooLongPassword := strings.Repeat("a", 73)

	hash, err := HashPassword(tooLongPassword)
	require.Empty(t, hash)
	require.Error(t, err)
	require.True(t, errors.Is(err, bcrypt.ErrPasswordTooLong))
	require.Contains(t, err.Error(), "hash password")
}

func TestCheckPassword_WrongPasswordReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct-password")
	require.NoError(t, err)

	err = CheckPassword(hash, "wrong-password")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	t.Parallel()

	err := CheckPassword("invalid-hash", "any-password")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrInvalidCredentials))
	require.Contains(t, err.Error(), "check password")
}
