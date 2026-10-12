package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/logocomune/maclookup-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/singleflight"
)

// Errors returned by VendorService. The HTTP layer maps them to status codes.
var (
	// ErrInvalidMAC means the input is not a MAC address or prefix.
	ErrInvalidMAC = errors.New("invalid MAC address: expected 6-12 hex digits, optionally separated by ':', '-', '.' or ' '")
	// ErrUpstreamAuth means maclookup.app rejected the configured API key.
	ErrUpstreamAuth = errors.New("upstream rejected the configured API key")
	// ErrUpstreamTimeout means maclookup.app did not answer in time.
	ErrUpstreamTimeout = errors.New("upstream timed out")
	// ErrUpstream covers every other upstream failure (network, bad payload, unexpected status).
	ErrUpstream = errors.New("upstream request failed")
)

// RateLimitedError is returned while the upstream quota is exhausted.
// RetryAfter is how long the caller should wait (0 if unknown).
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return "upstream rate limit exceeded"
}

// BadRequestError is returned when maclookup.app rejects the MAC as invalid.
type BadRequestError struct {
	Msg string
}

func (e *BadRequestError) Error() string {
	return e.Msg
}

// VendorInfo is the JSON representation of a full vendor lookup.
type VendorInfo struct {
	Found      bool   `json:"found"`
	MacPrefix  string `json:"mac_prefix,omitempty"`
	Company    string `json:"company,omitempty"`
	Address    string `json:"address,omitempty"`
	Country    string `json:"country,omitempty"`
	BlockStart string `json:"block_start,omitempty"`
	BlockEnd   string `json:"block_end,omitempty"`
	BlockSize  int    `json:"block_size,omitempty"`
	BlockType  string `json:"block_type,omitempty"`
	Updated    string `json:"updated,omitempty"`
	IsRandom   bool   `json:"is_random"`
	IsPrivate  bool   `json:"is_private"`
	Cached     bool   `json:"cached"`
}

// VendorName is the JSON representation of a company-name lookup.
type VendorName struct {
	Found     bool   `json:"found"`
	Company   string `json:"company,omitempty"`
	IsPrivate bool   `json:"is_private"`
	Cached    bool   `json:"cached"`
}

const (
	endpointLookup      = "lookup"
	endpointCompanyName = "company_name"
)

// VendorService resolves MAC addresses to vendors via maclookup.app.
//
// Vendor registrations change rarely while the free API quota is small
// (10 req/s at the time of writing), so results are cached, concurrent
// lookups of the same prefix are collapsed into one upstream call, and once
// the upstream reports 429 no further calls are made until its reset time.
type VendorService struct {
	client  *maclookup.Client
	metrics *serviceMetrics
	now     func() time.Time

	cacheTTL        time.Duration
	cacheMaxEntries int

	mu           sync.Mutex
	cache        map[string]cacheEntry
	blockedUntil time.Time

	group singleflight.Group
}

type cacheEntry struct {
	value   any // VendorInfo or VendorName
	expires time.Time
}

// NewVendorService creates a VendorService. Its metrics are registered on reg.
func NewVendorService(client *maclookup.Client, reg prometheus.Registerer, cacheTTL time.Duration, cacheMaxEntries int) *VendorService {
	s := &VendorService{
		client:          client,
		now:             time.Now,
		cacheTTL:        cacheTTL,
		cacheMaxEntries: cacheMaxEntries,
		cache:           make(map[string]cacheEntry),
	}
	s.metrics = newServiceMetrics(reg, s.cacheLen)

	return s
}

// Lookup returns the full vendor registration for mac.
func (s *VendorService) Lookup(mac string) (VendorInfo, error) {
	prefix, err := normalizeMAC(mac)
	if err != nil {
		return VendorInfo{}, err
	}

	v, cached, err := s.resolve(endpointLookup, prefix, func() (any, error) {
		r, err := s.client.Lookup(prefix)
		s.observeUpstream(endpointLookup, r.RespTime, r.RateLimit, err, r.Found)
		if err != nil {
			return nil, err
		}

		return VendorInfo{
			Found:      r.Found,
			MacPrefix:  r.MacPrefix,
			Company:    r.Company,
			Address:    r.Address,
			Country:    r.Country,
			BlockStart: r.BlockStart,
			BlockEnd:   r.BlockEnd,
			BlockSize:  r.BlockSize,
			BlockType:  r.BlockType,
			Updated:    r.Updated,
			IsRandom:   r.IsRand,
			IsPrivate:  r.IsPrivate,
		}, nil
	})
	if err != nil {
		return VendorInfo{}, err
	}

	info := v.(VendorInfo)
	info.Cached = cached

	return info, nil
}

// CompanyName returns only the vendor name for mac.
func (s *VendorService) CompanyName(mac string) (VendorName, error) {
	prefix, err := normalizeMAC(mac)
	if err != nil {
		return VendorName{}, err
	}

	v, cached, err := s.resolve(endpointCompanyName, prefix, func() (any, error) {
		r, err := s.client.CompanyName(prefix)
		s.observeUpstream(endpointCompanyName, r.RespTime, r.RateLimit, err, r.Found)
		if err != nil {
			return nil, err
		}

		return VendorName{
			Found:     r.Found,
			Company:   r.Company,
			IsPrivate: r.IsPrivate,
		}, nil
	})
	if err != nil {
		return VendorName{}, err
	}

	name := v.(VendorName)
	name.Cached = cached

	return name, nil
}

// resolve serves endpoint/prefix from the cache or calls fetch, collapsing
// concurrent calls for the same key. It reports whether the value was cached.
func (s *VendorService) resolve(endpoint, prefix string, fetch func() (any, error)) (any, bool, error) {
	key := endpoint + "/" + prefix

	if v, ok := s.cacheGet(key); ok {
		s.metrics.cacheRequests.WithLabelValues(endpoint, "hit").Inc()
		return v, true, nil
	}
	s.metrics.cacheRequests.WithLabelValues(endpoint, "miss").Inc()

	v, err, _ := s.group.Do(key, func() (any, error) {
		if wait := s.rateLimitWait(); wait > 0 {
			return nil, &RateLimitedError{RetryAfter: wait}
		}

		v, err := fetch()
		if err != nil {
			return nil, s.translateError(err)
		}
		s.cachePut(key, v)

		return v, nil
	})

	return v, false, err
}

// translateError maps maclookup-go's typed errors to this service's errors
// and records a rate-limit block when the upstream answers 429.
func (s *VendorService) translateError(err error) error {
	var (
		rateLimited *maclookup.RateLimitsExceeded
		badRequest  *maclookup.BadAPIRequest
		badKey      *maclookup.BadAPIKey
	)

	switch {
	case errors.As(err, &rateLimited):
		// Without a usable reset time, back off for a second rather than
		// hammering an upstream that just said no.
		until := rateLimited.Reset
		if !until.After(s.now()) {
			until = s.now().Add(time.Second)
		}
		s.mu.Lock()
		s.blockedUntil = until
		s.mu.Unlock()

		return &RateLimitedError{RetryAfter: until.Sub(s.now())}
	case errors.As(err, &badRequest):
		return &BadRequestError{Msg: badRequest.Error()}
	case errors.As(err, &badKey):
		return fmt.Errorf("%w: %v", ErrUpstreamAuth, redactURL(err))
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %v", ErrUpstreamTimeout, redactURL(err))
	default:
		return fmt.Errorf("%w: %v", ErrUpstream, redactURL(err))
	}
}

// redactURL drops the request URL from network errors. maclookup-go sends the
// API key as a query parameter, so the URL inside a *url.Error would leak it
// into logs.
func redactURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%s request: %w", urlErr.Op, urlErr.Err)
	}

	return err
}

func (s *VendorService) observeUpstream(endpoint string, respTime time.Duration, rl maclookup.RateLimit, err error, found bool) {
	s.metrics.upstreamRequests.WithLabelValues(endpoint, upstreamResult(err, found)).Inc()
	if respTime > 0 {
		s.metrics.upstreamDuration.WithLabelValues(endpoint).Observe(respTime.Seconds())
	}
	// maclookup-go reports -1 for a missing header and leaves the struct zeroed
	// when no response arrived at all; a positive Limit proves the headers
	// were actually read.
	if rl.Limit > 0 && rl.Remaining >= 0 {
		s.metrics.rateLimitRemaining.Set(float64(rl.Remaining))
	}
}

func upstreamResult(err error, found bool) string {
	var (
		rateLimited *maclookup.RateLimitsExceeded
		badRequest  *maclookup.BadAPIRequest
		badKey      *maclookup.BadAPIKey
	)

	switch {
	case err == nil && found:
		return "found"
	case err == nil:
		return "not_found"
	case errors.As(err, &rateLimited):
		return "rate_limited"
	case errors.As(err, &badRequest):
		return "bad_request"
	case errors.As(err, &badKey):
		return "bad_api_key"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "error"
	}
}

func (s *VendorService) rateLimitWait() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.blockedUntil.Sub(s.now())
}

func (s *VendorService) cacheGet(key string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.cache[key]
	if !ok {
		return nil, false
	}
	if !s.now().Before(e.expires) {
		delete(s.cache, key)
		return nil, false
	}

	return e.value, true
}

// cachePut stores v. When the cache is full, expired entries are purged
// first; if it is still full the value is simply not cached, which keeps
// memory bounded no matter how many distinct prefixes clients send.
func (s *VendorService) cachePut(key string, v any) {
	if s.cacheTTL <= 0 || s.cacheMaxEntries <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if len(s.cache) >= s.cacheMaxEntries {
		for k, e := range s.cache {
			if !now.Before(e.expires) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= s.cacheMaxEntries {
			return
		}
	}
	s.cache[key] = cacheEntry{value: v, expires: now.Add(s.cacheTTL)}
}

func (s *VendorService) cacheLen() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return float64(len(s.cache))
}

// normalizeMAC validates mac and returns the uppercase hex prefix that
// maclookup.app resolves, truncated exactly like maclookup-go does (9, 7 or
// 6 digits) so that equivalent inputs share one cache entry.
//
// Validating here matters: maclookup-go only strips separators and copies
// every other byte into the request path, so input such as "/" or "?" would
// otherwise end up in the upstream URL.
func normalizeMAC(mac string) (string, error) {
	var buf [12]byte
	n := 0

	for i := 0; i < len(mac); i++ {
		c := mac[i]
		switch {
		case c == ':' || c == '-' || c == '.' || c == ' ':
			continue
		case c >= '0' && c <= '9', c >= 'A' && c <= 'F':
		case c >= 'a' && c <= 'f':
			c -= 'a' - 'A'
		default:
			return "", ErrInvalidMAC
		}
		if n == len(buf) {
			return "", ErrInvalidMAC
		}
		buf[n] = c
		n++
	}

	switch {
	case n >= 9:
		n = 9
	case n >= 7:
		n = 7
	case n == 6:
	default:
		return "", ErrInvalidMAC
	}

	return string(buf[:n]), nil
}
