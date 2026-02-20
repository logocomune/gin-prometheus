package ginprom

// config is a configuration struct used for setting up service tracking options and behaviors.
type config struct {
	recordRequestSize   bool
	recordResponseSize  bool
	recordDuration      bool
	filterPath          func(string, string) bool
	pathAggregator      func(string, string, int) string
	aggregateStatusCode bool
	// markUnmatchedRoutes determines if unmatched routes should be marked with a special prefix
	markUnmatchedRoutes bool
	// unmatchedRoutesGrouping determines if unmatched routes should be grouped
	// into a single metric or tracked individually
	unmatchedRoutesGrouping bool
	// handleUnmatchedRoutes determines if unmatched routes should be handled specially
	handleUnmatchedRoutes bool

	// groupUnmatchedRoutes determines if unmatched routes should be grouped
	// into a single metric to prevent cardinality explosion
	groupUnmatchedRoutes bool
}

// Option is a functional option that configures the [Middleware] or
// [MiddlewareWithMetrics] behaviour.  Options are evaluated in order; later
// options override earlier ones when they affect the same field.
type Option func(*config)

// WithRecordRequestSize enables or disables recording of HTTP request body
// sizes.  When enabled (the default), every request is observed by the
// http_request_size_bytes histogram.
func WithRecordRequestSize(record bool) Option {
	return func(c *config) {
		c.recordRequestSize = record
	}
}

// WithRecordResponseSize enables or disables recording of HTTP response sizes.
// When enabled (the default), every response is observed by the
// http_response_size_bytes histogram.
func WithRecordResponseSize(record bool) Option {
	return func(c *config) {
		c.recordResponseSize = record
	}
}

// WithRecordDuration enables or disables recording of request durations.
// When enabled (the default), every request is observed by the
// http_request_duration_seconds histogram.
func WithRecordDuration(record bool) Option {
	return func(c *config) {
		c.recordDuration = record
	}
}

// WithFilterPath installs a custom filter function that decides, for each
// request, whether metrics should be skipped.  The function receives the Gin
// route pattern (first argument) and the raw URL path (second argument) and
// returns true when the request must be excluded from metrics collection.
//
// Example – skip any path that starts with "/internal":
//
//	ginprom.WithFilterPath(func(route, path string) bool {
//	    return strings.HasPrefix(path, "/internal")
//	})
func WithFilterPath(filter func(string, string) bool) Option {
	return func(c *config) {
		c.filterPath = filter
	}
}

// WithPathAggregator installs a custom function that maps a (route, path,
// statusCode) triple to the label value used in all four metrics.  The
// default implementation returns route when it is non-empty, "path_4xx" for
// 4xx unmatched requests, "path_5xx" for 5xx unmatched ones, and
// "missing_route" otherwise.
//
// Use this option to normalise dynamic segments that Gin does not capture as
// named parameters, or to further reduce metric cardinality.
func WithPathAggregator(aggregator func(string, string, int) string) Option {
	return func(c *config) {
		c.pathAggregator = aggregator
	}
}

// WithAggregateStatusCode controls status-code label granularity.  When
// enabled, individual codes are bucketed into class labels such as "2xx",
// "4xx", "5xx", reducing metric cardinality at the cost of less specific
// alerting.  Disabled by default.
func WithAggregateStatusCode(aggregate bool) Option {
	return func(c *config) {
		c.aggregateStatusCode = aggregate
	}
}

// WithFilterRoutes registers a list of exact Gin route patterns that should be
// excluded from metrics collection.  The match is performed against the
// registered pattern (e.g. "/health"), not the raw request URL.
//
// Example:
//
//	ginprom.WithFilterRoutes([]string{"/healthz", "/readyz", "/metrics"})
func WithFilterRoutes(routes []string) Option {
	return func(c *config) {
		routeToFilter := make(map[string]struct{})
		for _, r := range routes {
			routeToFilter[r] = struct{}{}
		}
		c.filterPath = func(route string, path string) bool {
			if _, ok := routeToFilter[route]; ok {
				return true
			}
			return false
		}
	}
}

// WithUnmatchedRouteHandling controls whether requests that do not match any
// registered Gin route are still counted in metrics.  When enabled (the
// default), such requests are grouped under an "/unmatched/*" label (see also
// [WithUnmatchedRouteGrouping]).  Disable this to silently drop all unmatched
// requests from metrics.
func WithUnmatchedRouteHandling(enabled bool) Option {
	return func(c *config) {
		c.handleUnmatchedRoutes = enabled
	}
}

// defaultConf initializes a default configuration instance for monitoring with pre-defined default settings.
func defaultConf(options ...Option) *config {
	return &config{
		recordRequestSize:  true,
		recordResponseSize: true,
		recordDuration:     true,
		filterPath:         func(string, string) bool { return false },
		pathAggregator: func(route string, path string, statusCode int) string {
			if route == "" {
				if statusCode >= 400 && statusCode < 500 {
					return "path_4xx"
				} else if statusCode >= 500 {
					return "path_5xx"
				}
				return "missing_route"
			}
			return route
		},
		aggregateStatusCode:   false,
		handleUnmatchedRoutes: true,
		groupUnmatchedRoutes:  true,
	}
}

// applyOpt processes a variadic list of Option functions and applies them to configure and return a new config instance.
func applyOpt(options ...Option) *config {
	conf := defaultConf()
	for _, option := range options {
		option(conf)
	}

	return conf
}
