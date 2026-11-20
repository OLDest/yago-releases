package store

import "yago/internal/mobile"

func SaveUpdateToken(device, token string) error {
	_, e := mobile.Call("updateCredentialSave", token)
	return e
}
func LoadUpdateToken(device string) (string, error) {
	return mobile.Call("updateCredentialRead", device)
}
func DeleteUpdateToken(device string) error {
	_, e := mobile.Call("updateCredentialDelete", device)
	return e
}
