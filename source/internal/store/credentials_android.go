package store

import (
	"encoding/json"
	"errors"
	"strings"
	"yago/internal/mobile"
	"yago/internal/music"
)

func Save(device string, t music.Token) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	method := "credentialSave"
	if strings.HasSuffix(device, ":zvuk") {
		method = "zvukCredentialSave"
	}
	_, e = mobile.Call(method, string(b))
	return e
}
func Load(device string) (music.Token, error) {
	var t music.Token
	method := "credentialRead"
	if strings.HasSuffix(device, ":zvuk") {
		method = "zvukCredentialRead"
	}
	s, e := mobile.Call(method, device)
	if e != nil {
		return t, e
	}
	if s == "" {
		return t, errors.New("no saved credential")
	}
	e = json.Unmarshal([]byte(s), &t)
	return t, e
}
func Delete(device string) error {
	method := "credentialDelete"
	if strings.HasSuffix(device, ":zvuk") {
		method = "zvukCredentialDelete"
	}
	_, e := mobile.Call(method, device)
	return e
}
