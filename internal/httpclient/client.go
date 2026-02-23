// Package httpclient provides an HTTP client that mimics Chrome's TLS
// fingerprint using uTLS.  This bypasses Cloudflare JA3-based blocking
// that rejects Go's default net/http TLS stack.
package httpclient

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// New returns an *http.Client whose TLS handshake looks like Chrome.
// Every HTTPS request gets a fresh TLS connection (no pooling) which is
// fine for CLI workloads that make only a handful of requests.
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &chromeTransport{dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}},
	}
}

// chromeTransport implements http.RoundTripper with uTLS Chrome fingerprint.
type chromeTransport struct {
	dialer *net.Dialer
}

func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return http.DefaultTransport.RoundTrip(req)
	}

	host := req.URL.Hostname()
	port := portFromURL(req.URL)
	addr := net.JoinHostPort(host, port)

	rawConn, err := t.dialer.DialContext(req.Context(), "tcp", addr)
	if err != nil {
		return nil, err
	}

	tlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"},
	}, utls.HelloChrome_Auto)

	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return nil, err
	}

	// Cloudflare strongly prefers HTTP/2.
	if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		h2t := &http2.Transport{
			DialTLSContext: func(_ context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
				return tlsConn, nil
			},
		}
		return h2t.RoundTrip(req)
	}

	// Fallback to HTTP/1.1.
	h1t := &http.Transport{
		DialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return tlsConn, nil
		},
		DisableKeepAlives: true,
	}
	return h1t.RoundTrip(req)
}

func portFromURL(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
