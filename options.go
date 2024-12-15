package ginprom

// config is a configuration struct used for setting up service tracking options and behaviors.
type config struct {
	recordRequests      bool
	recordRequestSize   bool
	recordResponseSize  bool
	recordDuration      bool
	groupedStatus       bool
	filterPath          func(string, string) bool
	pathAggregator      func(string, string, int) string
	aggregateStatusCode bool
}

// Option defines a function type used to modify the configuration of a service during initialization.
type Option func(*config)

// WithRecordRequests configures whether to enable or disable recording of requests in the configuration.
func WithRecordRequests(record bool) Option {
	return func(c *config) {
		c.recordRequests = record
	}
}

// WithRecordRequestSize enables or disables recording of request sizes in the configuration.
func WithRecordRequestSize(record bool) Option {
	return func(c *config) {
		c.recordRequestSize = record
	}
}

// WithRecordResponseSize sets whether the response size should be recorded in the configuration.
func WithRecordResponseSize(record bool) Option {
	return func(c *config) {
		c.recordResponseSize = record
	}
}

// WithRecordDuration sets whether to record the duration of operations in the configuration.
func WithRecordDuration(record bool) Option {
	return func(c *config) {
		c.recordDuration = record
	}
}

// WithGroupedStatus configures whether status codes should be grouped when collecting metrics.
func WithGroupedStatus(grouped bool) Option {
	return func(c *config) {
		c.groupedStatus = grouped
	}
}

// WithFilterPath sets a filter function to determine which paths should be included or excluded from certain operations.
func WithFilterPath(filter func(string, string) bool) Option {
	return func(c *config) {
		c.filterPath = filter
	}
}

// WithPathAggregator sets a custom path aggregator function to aggregate paths based on the provided arguments.
func WithPathAggregator(aggregator func(string, string, int) string) Option {
	return func(c *config) {
		c.pathAggregator = aggregator
	}
}

// WithAggregateStatusCode sets whether to aggregate request status codes in the configuration.
func WithAggregateStatusCode(aggregate bool) Option {
	return func(c *config) {
		c.aggregateStatusCode = aggregate
	}
}

// WithFilterRoutes creates an Option to configure a filter that allows tracking only specified routes in the service.
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

// defaultConf initializes a default configuration instance for monitoring with pre-defined default settings.
func defaultConf(options ...Option) *config {
	return &config{
		recordRequests:     true,
		recordRequestSize:  true,
		recordResponseSize: true,
		recordDuration:     true,
		groupedStatus:      true,
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
		aggregateStatusCode: false,
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
