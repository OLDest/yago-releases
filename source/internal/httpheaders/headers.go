// Package httpheaders keeps API and media requests separate. Media requests
// must never inherit account tokens, cookies, or signed API client identities.
package httpheaders

import (
	"net/http"
	"yago/internal/chromenet"
)

const UserAgent = chromenet.UserAgent

func Media(r *http.Request) {
	r.Header.Set("User-Agent", UserAgent)
	r.Header.Set("Accept", "*/*")
	// Compressed audio is already encoded. Keep byte counts and disk spool
	// contents identical to the media served by the CDN.
	r.Header.Set("Accept-Encoding", "identity")
}

func Yandex(r *http.Request, accept string) {
	r.Header.Set("User-Agent", UserAgent)
	r.Header.Set("Accept", accept)
	r.Header.Set("Accept-Language", "ru")
}
