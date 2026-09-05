package checker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func validateTargetHost(host string) error {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "metadata.google.internal" || host == "metadata" {
		return errors.New("cloud metadata targets are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && isCloudMetadataIP(ip) {
		return errors.New("cloud metadata targets are not allowed")
	}
	return nil
}

// Resolve once, validate every result, then dial the validated address itself.
// This closes the DNS rebinding gap between validating a URL and connecting.
func dialTargetContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err := validateTargetHost(host); err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isCloudMetadataIP(ip.IP) {
			return nil, errors.New("cloud metadata targets are not allowed")
		}
	}
	var dialer net.Dialer
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no addresses returned")
	}
	return nil, lastErr
}

var checkTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Dial monitoring targets directly, including LAN services. An environment
	// proxy would resolve targets remotely and bypass validation at connection time.
	transport.Proxy = nil
	transport.DialContext = dialTargetContext
	return transport
}()

func newCheckHTTPClient(timeout time.Duration, hasCredentials bool) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: checkTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			if err := ValidateURLTarget(req.URL.String()); err != nil {
				return err
			}
			if hasCredentials && len(via) > 0 &&
				(!strings.EqualFold(req.URL.Host, via[0].URL.Host) || req.URL.Scheme != via[0].URL.Scheme) {
				return fmt.Errorf("refusing to forward service credentials to a different origin")
			}
			return nil
		},
	}
}
