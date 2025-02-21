// Package cloudfrontgate is a plugin for CloudFrontGate.
package cloudfrontgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type contextKey string

const (
	// CTXHTTPTimeout is the context key for the HTTP timeout.
	CTXHTTPTimeout contextKey = "HTTPTimeout"
	// CTXTrustedIPs is the context key for the trusted IP ranges.
	CTXTrustedIPs contextKey = "TrustedIPs"
	// CFAPI is the CloudFront API URL.
	CFAPI = "https://d7uri8nf7uskq.cloudfront.net/tools/list-cloudfront-ips"
	// HTTPTimeoutDefault is the default HTTP timeout in seconds.
	HTTPTimeoutDefault = 5
)

// Config the plugin configuration.
type Config struct {
	// RefreshInterval is the interval between IP range updates
	RefreshInterval string `json:"refreshInterval,omitempty"`
	// AllowedIPs is a list of custom IP addresses or CIDR ranges that are allowed
	AllowedIPs []string `json:"allowedIPs,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		RefreshInterval: "24h",
	}
}

// CloudFrontGate is a CloudFrontGate plugin.
type CloudFrontGate struct {
	next http.Handler

	name string
	ips  *ipstore

	refreshInterval time.Duration
	trustedIPs      []net.IPNet
}

// New created a new CloudFrontGate plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	ips := newIPStore(CFAPI)

	refreshInterval, err := time.ParseDuration(config.RefreshInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refresh interval: %w", err)
	}

	trustedIPs, err := parseCIDRs(config.AllowedIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trusted IPs: %w", err)
	}

	ctxUpdate := createContext(ctx, HTTPTimeoutDefault, trustedIPs)

	if err := ips.Update(ctxUpdate); err != nil {
		return nil, fmt.Errorf("failed to update CloudFront IP ranges: %w", err)
	}

	cf := &CloudFrontGate{
		next: next,
		name: name,

		ips:             ips,
		trustedIPs:      trustedIPs,
		refreshInterval: refreshInterval,
	}

	go cf.refreshLoop(ctx)
	return cf, nil
}

func (cf *CloudFrontGate) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	remoteIP := net.ParseIP(strings.Split(req.RemoteAddr, ":")[0])
	if remoteIP == nil || !cf.ips.Contains(remoteIP) {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}

	cf.next.ServeHTTP(rw, req)
}

// refreshLoop periodically updates the IP ranges.
func (cf *CloudFrontGate) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(cf.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			ctxUpdate := createContext(ctx, HTTPTimeoutDefault, cf.trustedIPs)

			if err := cf.ips.Update(ctxUpdate); err != nil {
				log.Printf("Failed to update CloudFront IP ranges: %v", err)
			}
		}
	}
}

type ipstore struct {
	cfAPI string
	atomic.Value
}

func newIPStore(cfURL string) *ipstore {
	ips := &ipstore{
		cfAPI: cfURL,
	}
	ips.Store([]net.IPNet{})
	return ips
}

func (ips *ipstore) Contains(ip net.IP) bool {
	cidrs, ok := ips.Load().([]net.IPNet)
	if !ok {
		return false
	}
	for _, ipNet := range cidrs {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// Update fetches the latest CloudFront IP ranges and updates the store.
func (ips *ipstore) Update(ctx context.Context) error {
	trustedIPs, ok := ctx.Value(CTXTrustedIPs).([]net.IPNet)
	if !ok {
		return errors.New("invalid trusted IPs value")
	}

	fetchedCIDRs, err := ips.fetch(ctx)
	if err != nil {
		return err
	}

	cidrs := make([]net.IPNet, 0, len(trustedIPs)+len(fetchedCIDRs))
	cidrs = append(cidrs, trustedIPs...)
	cidrs = append(cidrs, fetchedCIDRs...)

	ips.Store(cidrs)
	return nil // Return nil if everything is successful
}

func (ips *ipstore) fetch(ctx context.Context) ([]net.IPNet, error) {
	timeout, ok := ctx.Value(CTXHTTPTimeout).(int) // Ensure timeout is of type int
	if !ok {
		return nil, errors.New("invalid timeout value")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ips.cfAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	// Check for a successful response
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status: %s", res.Status)
	}

	resp := CFResponse{}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return parseResponse(resp)
}

// CFResponse is a CloudFront API response.
type CFResponse struct {
	/*
		{
			"CLOUDFRONT_GLOBAL_IP_LIST": ["120.52.22.96/27", "205.251.249.0/24", "180.163.57.128/26", "204.246.168.0/22", "111.13.171.128/26", ... ],
			"CLOUDFRONT_REGIONAL_EDGE_IP_LIST": ["13.113.196.64/26", "13.113.203.0/24", "52.199.127.192/26", "13.124.199.0/24", "3.35.130.128/25", "..."]
		}
	*/
	GlobalIPList       []string `json:"CLOUDFRONT_GLOBAL_IP_LIST"`        //nolint:tagliatelle
	RegionalEdgeIPList []string `json:"CLOUDFRONT_REGIONAL_EDGE_IP_LIST"` //nolint:tagliatelle
}

func createContext(ctx context.Context, timeout int, trustedIPs []net.IPNet) context.Context {
	ctx = context.WithValue(ctx, CTXHTTPTimeout, timeout)
	return context.WithValue(ctx, CTXTrustedIPs, trustedIPs)
}

func parseResponse(resp CFResponse) ([]net.IPNet, error) {
	globalIPList, err := parseCIDRs(resp.GlobalIPList)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CLOUDFRONT_GLOBAL_IP_LIST CIDRs: %w", err)
	}
	regionalEdgeIPList, err := parseCIDRs(resp.RegionalEdgeIPList)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CLOUDFRONT_REGIONAL_EDGE_IP_LIST CIDRs: %w", err)
	}
	return append(globalIPList, regionalEdgeIPList...), nil
}

func parseCIDRs(ips []string) ([]net.IPNet, error) {
	trustedIPs := make([]net.IPNet, 0, len(ips))
	for _, ip := range ips {
		if !strings.Contains(ip, "/") {
			ip = fmt.Sprintf("%s/32", ip)
		}
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CIDR: %w", err)
		}
		trustedIPs = append(trustedIPs, *ipNet)
	}
	return trustedIPs, nil
}
