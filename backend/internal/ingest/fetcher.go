package ingest

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const (
	MaxHTMLBytes       = 10 * 1024 * 1024 // 10MB
	MaxImageBytes      = 25 * 1024 * 1024 // 25MB
	DefaultUserAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	DefaultHTMLAccept  = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	DefaultImageAccept = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
)

var restrictedCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",          // Current network (RFC 1122)
		"100.64.0.0/10",      // Shared Address Space / CGNAT (RFC 6598)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // TEST-NET-1 (RFC 5737)
		"198.18.0.0/15",      // Network interconnect device benchmark (RFC 2544)
		"198.51.100.0/24",    // TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // TEST-NET-3 (RFC 5737)
		"240.0.0.0/4",        // Reserved for future use (RFC 1112)
		"255.255.255.255/32", // Limited broadcast (RFC 919)
	}
	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			restrictedCIDRs = append(restrictedCIDRs, block)
		}
	}
}

// isPrivateOrRestrictedIP returns true if the IP address is private, loopback, link-local, multicast, or in a reserved range.
func isPrivateOrRestrictedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, block := range restrictedCIDRs {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

type HTTPFetcher struct {
	client         *retryablehttp.Client
	AllowLocalhost bool
}

func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	fetcher := &HTTPFetcher{}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", addr, err)
			}

			if !fetcher.AllowLocalhost {
				if ip := net.ParseIP(host); ip != nil {
					if isPrivateOrRestrictedIP(ip) {
						return nil, fmt.Errorf("access to private or restricted IP blocked: %s", ip)
					}
					return dialer.DialContext(ctx, network, addr)
				}

				ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, fmt.Errorf("DNS resolution failed for host %s: %w", host, err)
				}
				if len(ips) == 0 {
					return nil, fmt.Errorf("no IP address found for host: %s", host)
				}
				for _, ip := range ips {
					if isPrivateOrRestrictedIP(ip) {
						return nil, fmt.Errorf("access to private or restricted IP blocked for host %s: %s", host, ip)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
			}

			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 2
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 3 * time.Second
	client.Logger = nil
	client.HTTPClient.Timeout = timeout
	client.HTTPClient.Transport = transport

	// Do not retry on SSRF blocks
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "blocked") || strings.Contains(errStr, "private") || strings.Contains(errStr, "denied") {
				return false, nil
			}
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	// Protect against redirects to private/restricted addresses
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if !fetcher.AllowLocalhost {
			host := req.URL.Hostname()
			if ip := net.ParseIP(host); ip != nil {
				if isPrivateOrRestrictedIP(ip) {
					return fmt.Errorf("redirect to private or restricted IP blocked: %s", ip)
				}
			} else {
				ips, err := net.DefaultResolver.LookupIP(req.Context(), "ip", host)
				if err != nil {
					return fmt.Errorf("DNS resolution failed for redirect host %s: %w", host, err)
				}
				for _, ip := range ips {
					if isPrivateOrRestrictedIP(ip) {
						return fmt.Errorf("redirect to private or restricted IP blocked for host %s: %s", host, ip)
					}
				}
			}
		}
		return nil
	}

	fetcher.client = client
	return fetcher
}

func (f *HTTPFetcher) validateURL(ctx context.Context, rawURL string) (*urlpkg.URL, error) {
	parsedURL, err := urlpkg.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q: only http and https are allowed", parsedURL.Scheme)
	}
	if !f.AllowLocalhost {
		host := parsedURL.Hostname()
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateOrRestrictedIP(ip) {
				return nil, fmt.Errorf("access to private or restricted IP blocked: %s", ip)
			}
		} else {
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed for host %s: %w", host, err)
			}
			for _, ip := range ips {
				if isPrivateOrRestrictedIP(ip) {
					return nil, fmt.Errorf("access to private or restricted IP blocked for host %s: %s", host, ip)
				}
			}
		}
	}
	return parsedURL, nil
}

func (f *HTTPFetcher) fetch(ctx context.Context, rawURL string, userAgent string, accept string, maxBytes int64, isHTML bool, errMsg string) ([]byte, error) {
	if _, err := f.validateURL(ctx, rawURL); err != nil {
		return nil, fmt.Errorf("fetch %s failed: %w", errMsg, err)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request failed: %w", errMsg, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s failed: %w", errMsg, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s returned status: %d", errMsg, resp.StatusCode)
	}

	if isHTML {
		contentType := resp.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			mediaType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			return nil, fmt.Errorf("fetch %s failed: unsupported content-type: %q", errMsg, contentType)
		}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s response body failed: %w", errMsg, err)
	}

	return data, nil
}

func (f *HTTPFetcher) FetchHTML(ctx context.Context, rawURL string) ([]byte, error) {
	return f.fetch(ctx, rawURL, DefaultUserAgent, DefaultHTMLAccept, MaxHTMLBytes, true, "page")
}

func (f *HTTPFetcher) FetchImage(ctx context.Context, imgURL string) ([]byte, error) {
	return f.fetch(ctx, imgURL, DefaultUserAgent, DefaultImageAccept, MaxImageBytes, false, "image")
}
