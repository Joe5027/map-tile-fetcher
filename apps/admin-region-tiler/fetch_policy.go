package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	urlpkg "net/url"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type FetchErrorCategory string

const (
	FetchErrorUnknown  FetchErrorCategory = "unknown"
	FetchErrorThrottle FetchErrorCategory = "throttle"
	FetchErrorBlocked  FetchErrorCategory = "blocked"
	FetchErrorUpstream FetchErrorCategory = "upstream"
	FetchErrorProxy    FetchErrorCategory = "proxy"
	FetchErrorNetwork  FetchErrorCategory = "network"
	FetchErrorContent  FetchErrorCategory = "content"
)

type FetchPolicy struct {
	Name           string
	UserAgent      string
	Referer        string
	Headers        map[string]string
	Proxies        []string
	RotateHosts    bool
	WorkerCount    int
	BaseDelayMS    int
	TimeJitterMS   int
	MaxRetries     int
	MaxRetriesSet  bool
	RetryPasses    int
	RetryPassesSet bool
	RetryStatuses  map[int]struct{}
}

type sourcePolicyConfig struct {
	Name               string            `mapstructure:"name"`
	HostContains       string            `mapstructure:"host_contains"`
	SourceNameContains string            `mapstructure:"source_name_contains"`
	UserAgent          string            `mapstructure:"user_agent"`
	Referer            string            `mapstructure:"referer"`
	WorkerCount        int               `mapstructure:"worker_count"`
	BaseDelayMS        int               `mapstructure:"base_delay_ms"`
	TimeJitterMS       int               `mapstructure:"time_jitter_ms"`
	MaxRetries         *int              `mapstructure:"max_retries"`
	RetryPasses        *int              `mapstructure:"retry_passes"`
	RotateHosts        *bool             `mapstructure:"rotate_hosts"`
	Proxies            []string          `mapstructure:"proxies"`
	RetryStatuses      []int             `mapstructure:"retry_statuses"`
	Headers            map[string]string `mapstructure:"headers"`
}

type proxyRotator struct {
	proxies []string
	index   int
	mu      sync.Mutex
}

type FetchFailure struct {
	URL        string
	Category   FetchErrorCategory
	StatusCode int
	Proxy      string
	Err        error
}

func (e *FetchFailure) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("fetch %s failed", e.URL)}
	if e.Category != "" && e.Category != FetchErrorUnknown {
		parts = append(parts, fmt.Sprintf("category=%s", e.Category))
	}
	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if strings.TrimSpace(e.Proxy) != "" {
		parts = append(parts, fmt.Sprintf("proxy=%s", e.Proxy))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ", ")
}

func (e *FetchFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newProxyRotator(proxies []string) *proxyRotator {
	cleaned := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		proxy = strings.TrimSpace(proxy)
		if proxy == "" {
			continue
		}
		cleaned = append(cleaned, proxy)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return &proxyRotator{proxies: cleaned}
}

func (r *proxyRotator) Next() string {
	if r == nil || len(r.proxies) == 0 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	proxy := r.proxies[r.index%len(r.proxies)]
	r.index = (r.index + 1) % len(r.proxies)
	return proxy
}

func defaultFetchPolicy(url string, sourceName string) FetchPolicy {
	policy := FetchPolicy{
		Name:        "default",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headers:     map[string]string{"Connection": "close"},
		RotateHosts: false,
	}

	host := strings.ToLower(hostnameOf(url))
	if strings.HasSuffix(host, ".tianditu.gov.cn") || strings.Contains(strings.ToLower(sourceName), "天地图") {
		policy.Name = "tianditu"
		policy.Referer = "https://map.tianditu.gov.cn"
		policy.RotateHosts = true
		policy.WorkerCount = 1
		policy.BaseDelayMS = 120
		policy.TimeJitterMS = 120
		policy.MaxRetries = 0
		policy.MaxRetriesSet = true
		policy.RetryPasses = 1
		policy.RetryPassesSet = true
		policy.RetryStatuses = map[int]struct{}{
			http.StatusTeapot:             {},
			http.StatusTooManyRequests:    {},
			http.StatusBadGateway:         {},
			http.StatusServiceUnavailable: {},
			http.StatusGatewayTimeout:     {},
		}
	}

	applyConfiguredSourcePolicy(&policy, url, sourceName)
	return policy
}

func applyConfiguredSourcePolicy(policy *FetchPolicy, rawURL string, sourceName string) {
	if policy == nil {
		return
	}
	var configs []sourcePolicyConfig
	if err := viper.UnmarshalKey("source_policies", &configs); err != nil || len(configs) == 0 {
		return
	}

	host := strings.ToLower(hostnameOf(rawURL))
	loweredSource := strings.ToLower(sourceName)
	for _, cfg := range configs {
		hostMatch := strings.TrimSpace(cfg.HostContains) == "" || strings.Contains(host, strings.ToLower(strings.TrimSpace(cfg.HostContains)))
		sourceMatch := strings.TrimSpace(cfg.SourceNameContains) == "" || strings.Contains(loweredSource, strings.ToLower(strings.TrimSpace(cfg.SourceNameContains)))
		if !hostMatch || !sourceMatch {
			continue
		}

		if strings.TrimSpace(cfg.Name) != "" {
			policy.Name = strings.TrimSpace(cfg.Name)
		}
		if strings.TrimSpace(cfg.UserAgent) != "" {
			policy.UserAgent = strings.TrimSpace(cfg.UserAgent)
		}
		if strings.TrimSpace(cfg.Referer) != "" {
			policy.Referer = strings.TrimSpace(cfg.Referer)
		}
		if cfg.WorkerCount > 0 {
			policy.WorkerCount = cfg.WorkerCount
		}
		if cfg.BaseDelayMS > 0 {
			policy.BaseDelayMS = cfg.BaseDelayMS
		}
		if cfg.TimeJitterMS > 0 {
			policy.TimeJitterMS = cfg.TimeJitterMS
		}
		if cfg.MaxRetries != nil && *cfg.MaxRetries >= 0 {
			policy.MaxRetries = *cfg.MaxRetries
			policy.MaxRetriesSet = true
		}
		if cfg.RetryPasses != nil && *cfg.RetryPasses >= 0 {
			policy.RetryPasses = *cfg.RetryPasses
			policy.RetryPassesSet = true
		}
		if cfg.RotateHosts != nil {
			policy.RotateHosts = *cfg.RotateHosts
		}
		if len(cfg.Proxies) > 0 {
			policy.Proxies = append([]string(nil), cfg.Proxies...)
		}
		if len(cfg.Headers) > 0 {
			if policy.Headers == nil {
				policy.Headers = make(map[string]string)
			}
			for key, value := range cfg.Headers {
				if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
					continue
				}
				policy.Headers[key] = value
			}
		}
		if len(cfg.RetryStatuses) > 0 {
			policy.RetryStatuses = make(map[int]struct{}, len(cfg.RetryStatuses))
			for _, status := range cfg.RetryStatuses {
				policy.RetryStatuses[status] = struct{}{}
			}
		}
		return
	}
}

func classifyFetchFailure(tileURL string, proxy string, err error) error {
	if err == nil {
		return nil
	}

	if failure := extractFetchFailure(err); failure != nil {
		return failure
	}

	category := FetchErrorUnknown
	statusCode := extractHTTPStatusCode(err)
	switch {
	case statusCode == http.StatusTeapot:
		category = FetchErrorBlocked
	case statusCode == http.StatusTooManyRequests:
		category = FetchErrorThrottle
	case statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout:
		category = FetchErrorUpstream
	case statusCode > 0:
		category = FetchErrorContent
	case strings.TrimSpace(proxy) != "" && isProxyishError(err):
		category = FetchErrorProxy
	case isNetworkishError(err):
		category = FetchErrorNetwork
	}

	return &FetchFailure{
		URL:        tileURL,
		Category:   category,
		StatusCode: statusCode,
		Proxy:      strings.TrimSpace(proxy),
		Err:        err,
	}
}

func extractFetchFailure(err error) *FetchFailure {
	if err == nil {
		return nil
	}
	var failure *FetchFailure
	if errors.As(err, &failure) {
		return failure
	}
	return nil
}

func extractHTTPStatusCode(err error) int {
	if err == nil {
		return 0
	}
	var statusErr HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	if failure := extractFetchFailure(err); failure != nil {
		return failure.StatusCode
	}
	return 0
}

func fetchErrorCategory(err error) FetchErrorCategory {
	if failure := extractFetchFailure(err); failure != nil {
		return failure.Category
	}
	return FetchErrorUnknown
}

func hostnameOf(rawURL string) string {
	parsed, err := urlpkg.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func isProxyishError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "proxy") || strings.Contains(msg, "tunnel")
}

func isNetworkishError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
