package httpx

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

func baseTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func Plain(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: baseTransport()}
}

func Insecure(timeout time.Duration) *http.Client {
	tr := baseTransport()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	return &http.Client{Timeout: timeout, Transport: tr}
}

func CDNTransfer(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			ForceAttemptHTTP2:   false,
			MaxIdleConns:        256,
			MaxIdleConnsPerHost: 256, // keep every worker's connection warm (no re-handshake between chunks)
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig:     &tls.Config{NextProtos: []string{"http/1.1"}},
		},
	}
}

func Chrome(timeout time.Duration) *http.Client {
	dialTLS := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		raw, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		uconn := utls.UClient(raw, &utls.Config{
			ServerName: host,
			NextProtos: []string{"http/1.1"},
		}, utls.HelloChrome_Auto)
		if err := uconn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return uconn, nil
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:             nil,
			DialTLSContext:    dialTLS,
			ForceAttemptHTTP2: false,
		},
	}
}
