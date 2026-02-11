package model

type User struct {
	ID           int64  `json:"id"`
	Login        string `json:"login"`
	PasswordHash string `json:"-"`
}

type RegisterUser struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}
