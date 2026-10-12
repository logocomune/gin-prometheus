package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ginprom "github.com/logocomune/gin-prometheus"
	"github.com/logocomune/maclookup-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeUpstream mimics the maclookup.app API v2. The prefix in the path picks
// the scenario, so every test runs offline and deterministically.
type fakeUpstream struct {
	srv   *httptest.Server
	calls atomic.Int64
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeUpstream) serve(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)

	rest := strings.TrimPrefix(r.URL.Path, "/v2/macs/")
	prefix, isName := strings.CutSuffix(rest, "/company/name")

	w.Header().Set("X-RateLimit-Limit", "10")
	w.Header().Set("X-RateLimit-Remaining", "7")

	switch prefix {
	case "AAAAAA": // quota exhausted
		reset := time.Now().Add(30 * time.Second).Unix()
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	case "BBBBBB": // upstream considers the MAC invalid
		w.WriteHeader(http.StatusBadRequest)
		if isName {
			io.WriteString(w, "Invalid MAC")
		} else {
			io.WriteString(w, `{"success":false,"error":"Invalid MAC","errorCode":101}`)
		}
		return
	case "CCCCCC": // slower than the client timeout
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		return
	case "DDDDDD": // rejected API key
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"success":false,"error":"Unauthorized: invalid api key secret123","errorCode":401}`)
		return
	}

	if isName {
		switch prefix {
		case "001122":
			io.WriteString(w, "*PRIVATE*")
		case "010000":
			io.WriteString(w, "*NO COMPANY*")
		default:
			io.WriteString(w, "XEROX CORPORATION")
		}
		return
	}

	if prefix == "010000" {
		io.WriteString(w, `{"success":true,"found":false}`)
		return
	}
	io.WriteString(w, `{"success":true,"found":true,"macPrefix":"`+prefix+`","company":"XEROX CORPORATION",`+
		`"address":"M/S 105-50C, WEBSTER NY 14580, US","country":"US","blockStart":"000000","blockEnd":"000000FFFFFF",`+
		`"blockSize":16777215,"blockType":"MA-L","updated":"2015-11-17","isRand":false,"isPrivate":false}`)
}

type testEnv struct {
	upstream *fakeUpstream
	svc      *VendorService
	reg      *prometheus.Registry
	router   *gin.Engine
}

func newTestEnv(t *testing.T, cacheTTL time.Duration, cacheMax int) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	up := newFakeUpstream(t)
	client := maclookup.New()
	client.WithPrefixURI(up.srv.URL)
	client.WithTimeout(200 * time.Millisecond)

	reg := prometheus.NewRegistry()
	svc := NewVendorService(client, reg, cacheTTL, cacheMax)

	return &testEnv{
		upstream: up,
		svc:      svc,
		reg:      reg,
		router:   newRouter(svc, ginprom.GetMetricHandler()),
	}
}

func (e *testEnv) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}

	return v
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "00:00:00:00:00:00", want: "000000000"},
		{in: "aa-bb-cc", want: "AABBCC"},
		{in: "aabb.ccdd.eeff", want: "AABBCCDDE"},
		{in: "AABBCCD", want: "AABBCCD"},
		{in: "AABBCCDD", want: "AABBCCD"},
		{in: "AA BB CC", want: "AABBCC"},
		{in: "", wantErr: true},
		{in: "AABBC", wantErr: true},
		{in: "AABBCCDDEEFF00", wantErr: true},
		{in: "GG:BB:CC", wantErr: true},
		{in: "00/00/00", wantErr: true},
		{in: "000000?apiKey=x", wantErr: true},
	}
	for _, tt := range tests {
		got, err := normalizeMAC(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("normalizeMAC(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("normalizeMAC(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestLookup_FoundThenCached(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/00:00:00:12:34:56")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}
	info := decode[VendorInfo](t, w)
	if !info.Found || info.Company != "XEROX CORPORATION" || info.BlockType != "MA-L" || info.Cached {
		t.Fatalf("unexpected first response: %+v", info)
	}

	// Different notation of the same prefix must hit the cache.
	w = e.get(t, "/api/v1/vendors/000000123456")
	if info := decode[VendorInfo](t, w); !info.Cached {
		t.Fatalf("second lookup not served from cache: %+v", info)
	}
	if n := e.upstream.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}

	if got := testutil.ToFloat64(e.svc.metrics.upstreamRequests.WithLabelValues(endpointLookup, "found")); got != 1 {
		t.Errorf("upstream found counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(e.svc.metrics.cacheRequests.WithLabelValues(endpointLookup, "hit")); got != 1 {
		t.Errorf("cache hit counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(e.svc.metrics.rateLimitRemaining); got != 7 {
		t.Errorf("rate limit remaining = %v, want 7", got)
	}
}

func TestLookup_NotFound(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/01:00:00")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if info := decode[VendorInfo](t, w); info.Found {
		t.Fatalf("expected found=false: %+v", info)
	}
}

func TestLookup_InvalidMACNeverReachesUpstream(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	tests := map[string]int{
		"zz":                http.StatusBadRequest,
		"00:00":             http.StatusBadRequest,
		"000000%3FapiKey=x": http.StatusBadRequest,
		// The escaped slashes are decoded before routing, so no route matches.
		"00%2F00%2F00": http.StatusNotFound,
	}
	for mac, want := range tests {
		if w := e.get(t, "/api/v1/vendors/"+mac); w.Code != want {
			t.Errorf("%q: status = %d, want %d", mac, w.Code, want)
		}
	}
	if n := e.upstream.calls.Load(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestLookup_UpstreamBadRequest(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/BBBBBB")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if body := decode[map[string]string](t, w); body["error"] != "invalid mac" {
		t.Fatalf("error = %q", body["error"])
	}
}

func TestRateLimit_BlocksFurtherUpstreamCalls(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/AAAAAA")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	retry, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || retry < 1 || retry > 30 {
		t.Fatalf("Retry-After = %q, want 1..30", w.Header().Get("Retry-After"))
	}

	// While blocked, other prefixes and endpoints must not call upstream.
	for _, path := range []string{"/api/v1/vendors/000000", "/api/v1/vendors/000000/name"} {
		if w := e.get(t, path); w.Code != http.StatusTooManyRequests {
			t.Errorf("%s: status = %d, want 429", path, w.Code)
		}
	}
	if n := e.upstream.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}

	// After the reset time upstream is used again.
	e.svc.now = func() time.Time { return time.Now().Add(time.Minute) }
	if w := e.get(t, "/api/v1/vendors/000000"); w.Code != http.StatusOK {
		t.Fatalf("after reset: status = %d, want 200", w.Code)
	}
}

func TestLookup_Timeout(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/CCCCCC")
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504; body %s", w.Code, w.Body)
	}
	if got := testutil.ToFloat64(e.svc.metrics.upstreamRequests.WithLabelValues(endpointLookup, "timeout")); got != 1 {
		t.Errorf("timeout counter = %v, want 1", got)
	}
}

func TestLookup_UpstreamErrorsDoNotContainAPIKey(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)
	e.svc.client.WithAPIKey("topsecretkey")

	_, err := e.svc.Lookup("CCCCCC") // times out: a *url.Error carrying the full URL
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "topsecretkey") {
		t.Fatalf("error leaks the API key: %v", err)
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error lost its cause: %v", err)
	}
}

func TestRateLimitGauge_UnknownUntilReported(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)
	gauge := e.svc.metrics.rateLimitRemaining

	if got := testutil.ToFloat64(gauge); got != -1 {
		t.Fatalf("initial gauge = %v, want -1", got)
	}
	// A timeout produces no response headers and must not touch the gauge.
	e.svc.Lookup("CCCCCC")
	if got := testutil.ToFloat64(gauge); got != -1 {
		t.Fatalf("gauge after timeout = %v, want -1", got)
	}
	e.svc.Lookup("000000")
	if got := testutil.ToFloat64(gauge); got != 7 {
		t.Fatalf("gauge after success = %v, want 7", got)
	}
}

func TestLookup_BadAPIKeyIsNotLeaked(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	w := e.get(t, "/api/v1/vendors/DDDDDD")
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if strings.Contains(w.Body.String(), "secret123") || strings.Contains(strings.ToLower(w.Body.String()), "key") {
		t.Fatalf("response leaks upstream details: %s", w.Body)
	}
}

func TestCompanyName(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	tests := []struct {
		mac         string
		wantStatus  int
		wantCompany string
		wantPrivate bool
	}{
		{mac: "00:00:00", wantStatus: http.StatusOK, wantCompany: "XEROX CORPORATION"},
		{mac: "00:11:22", wantStatus: http.StatusOK, wantPrivate: true},
		{mac: "01:00:00", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		w := e.get(t, "/api/v1/vendors/"+tt.mac+"/name")
		if w.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d", tt.mac, w.Code, tt.wantStatus)
			continue
		}
		name := decode[VendorName](t, w)
		if name.Company != tt.wantCompany || name.IsPrivate != tt.wantPrivate {
			t.Errorf("%s: got %+v", tt.mac, name)
		}
	}
}

func TestConcurrentLookupsShareOneUpstreamCall(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Lookup("00:00:00"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if n := e.upstream.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestCache_ExpiresAndStaysBounded(t *testing.T) {
	e := newTestEnv(t, time.Minute, 1)

	if _, err := e.svc.Lookup("000000"); err != nil {
		t.Fatal(err)
	}
	// Cache is full: a second prefix is served but not cached.
	for i := 0; i < 2; i++ {
		if _, err := e.svc.Lookup("000001"); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.upstream.calls.Load(); n != 3 {
		t.Fatalf("upstream calls = %d, want 3", n)
	}
	if got := e.svc.cacheLen(); got != 1 {
		t.Fatalf("cache entries = %v, want 1", got)
	}

	// After the TTL the first entry is gone and gets refetched.
	e.svc.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	info, err := e.svc.Lookup("000000")
	if err != nil {
		t.Fatal(err)
	}
	if info.Cached {
		t.Fatal("expired entry was served from cache")
	}
}

func TestMetricsEndpointUsesRoutePattern(t *testing.T) {
	e := newTestEnv(t, time.Hour, 100)
	e.get(t, "/api/v1/vendors/00:00:00")
	e.get(t, "/healthz")

	w := e.get(t, "/metrics")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `path="/api/v1/vendors/:mac"`) {
		t.Fatal("http metrics are not labelled with the route pattern")
	}
	if strings.Contains(body, `path="/api/v1/vendors/00:00:00"`) {
		t.Fatal("raw MAC leaked into a metric label")
	}
	if strings.Contains(body, `path="/healthz"`) || strings.Contains(body, `path="/metrics"`) {
		t.Fatal("filtered routes appear in metrics")
	}
}
