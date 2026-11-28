package musify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"runtime"
	"time"
)

func (c *Client) SignedIn() bool { return c.token.Load() != nil }
func (c *Client) SetSession(token string) {
	if token == "" {
		c.token.Store(nil)
	} else {
		c.token.Store(&token)
	}
}

func (c *Client) Login(ctx context.Context, login, password string) (string, error) {
	body, err := json.Marshal(struct {
		Login      string `json:"login"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
	}{login, password, "YaGo " + runtime.GOOS})
	if err != nil {
		return "", errors.New("Не удалось подготовить вход Musify")
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"auth/login", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("Не удалось подготовить вход Musify")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Install-Id", c.installID)
	start := time.Now()
	res, err := client.Do(req)
	if err != nil {
		if Diagnostics == "1" {
			log.Print("musify op=auth/login outcome=transport_error")
		}
		return "", errors.New("Не удалось связаться с Musify")
	}
	defer res.Body.Close()
	if Diagnostics == "1" {
		log.Printf("musify op=auth/login status=%d elapsed_ms=%d", res.StatusCode, time.Since(start).Milliseconds())
	}
	if res.StatusCode != http.StatusOK {
		return "", &HTTPError{Status: res.StatusCode}
	}
	var result struct{ Token string }
	if json.NewDecoder(res.Body).Decode(&result) != nil || result.Token == "" {
		if Diagnostics == "1" {
			log.Print("musify op=auth/login outcome=invalid_session_response")
		}
		return "", errors.New("Musify не вернул сессию")
	}
	return result.Token, nil
}
