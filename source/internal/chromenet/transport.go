// Package chromenet adapts the Chrome TLS/HTTP profile to YaGo's existing HTTP clients.
package chromenet

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/154.0.0.0 Safari/537.36"

// Transport retains the caller's redirect and token policies while using a Chrome profile on the wire.
type Transport struct {
	mu      sync.Mutex
	clients map[string]tlsclient.HttpClient
}

// NewTransport creates an isolated connection pool without shared cookies or account headers.
func NewTransport() *Transport {
	return &Transport{clients: make(map[string]tlsclient.HttpClient)}
}

// clientFor respects the standard environment proxy and NO_PROXY selection for each request.
func (t *Transport) clientFor(req *http.Request) (tlsclient.HttpClient, error) {
	proxy, err := http.ProxyFromEnvironment(req)
	if err != nil {
		return nil, err
	}
	key := ""
	if proxy != nil {
		key = proxy.String()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if client := t.clients[key]; client != nil {
		return client, nil
	}
	idle := 30 * time.Second
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(profiles.Chrome_154),
		tlsclient.WithRandomTLSExtensionOrder(),
		tlsclient.WithNotFollowRedirects(),
		tlsclient.WithDisableHttp3(),
		tlsclient.WithTimeoutMilliseconds(0),
		tlsclient.WithDialer(net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}),
		tlsclient.WithTransportOptions(&tlsclient.TransportOptions{
			MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: &idle,
			ResponseHeaderTimeout: 20 * time.Second,
		}),
	}
	if key != "" {
		options = append(options, tlsclient.WithProxyUrl(key))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("Chrome transport initialization failed: %w", err)
	}
	t.clients[key] = client
	return client, nil
}

// RoundTrip streams the original body and preserves cancellation without buffering audio in memory.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	client, err := t.clientFor(req)
	if err != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, err
	}
	r, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, err
	}
	r.Header = fhttp.Header(req.Header.Clone())
	r.Header.Set("User-Agent", UserAgent)
	order := []string{"host", "connection", "content-length", "user-agent", "accept", "accept-language", "accept-encoding", "origin", "referer", "content-type", "authorization", "x-auth-token"}
	for name := range req.Header {
		lower := strings.ToLower(name)
		found := false
		for _, existing := range order {
			if existing == lower {
				found = true
				break
			}
		}
		if !found {
			order = append(order, lower)
		}
	}
	r.Header[fhttp.HeaderOrderKey] = order
	r.Host = req.Host
	r.ContentLength = req.ContentLength
	r.GetBody = req.GetBody
	r.Close = req.Close
	r.TransferEncoding = append([]string(nil), req.TransferEncoding...)
	r.Trailer = fhttp.Header(req.Trailer.Clone())
	response, err := client.Do(r)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		Status: response.Status, StatusCode: response.StatusCode,
		Proto: response.Proto, ProtoMajor: response.ProtoMajor, ProtoMinor: response.ProtoMinor,
		Header: http.Header(response.Header), Body: response.Body, ContentLength: response.ContentLength,
		TransferEncoding: response.TransferEncoding, Close: response.Close,
		Uncompressed: response.Uncompressed, Trailer: http.Header(response.Trailer), Request: req,
	}, nil
}

// CloseIdleConnections releases this transport's unused connections without closing active streams.
func (t *Transport) CloseIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, client := range t.clients {
		client.CloseIdleConnections()
	}
}
