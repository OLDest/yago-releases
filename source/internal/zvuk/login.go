package zvuk

import "yago/internal/music"

type LoginResult struct {
	Token   string
	Account music.Account
}

const loginURL = "https://id.zvuk.com/?redirect_url=https%3A%2F%2Fzvuk.com%2F"
