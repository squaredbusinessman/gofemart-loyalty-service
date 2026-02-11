package auth

import "errors"

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInvalidToken = errors.New("invalid token")
var ErrExpiredToken = errors.New("token expired")
