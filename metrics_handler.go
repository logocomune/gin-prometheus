package ginprom

import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

type handlerConfig struct {
	username string
	password string
}

// Option defines a function type used to modify the configuration of a service during initialization.
type HandlerOption func(*handlerConfig)

func WithBasicAuth(username, password string) HandlerOption {
	return func(c *handlerConfig) {
		c.username = username
		c.password = password
	}
}

// GetMetricHandler returns an HTTP handler for exposing Prometheus metrics collected by the prometheus/promhttp package.
func GetMetricHandler(opt ...HandlerOption) http.Handler {
	conf := handlerConfig{}
	for _, o := range opt {
		o(&conf)
	}
	if (conf.username != "") && (conf.password != "") {
		return withBasicAuth(promhttp.Handler(), conf.username, conf.password)
	}
	return promhttp.Handler()
}

// getMetricHandlerWithBasicAuth returns an HTTP handler for Prometheus metrics with Basic Authentication enabled.
func getMetricHandlerWithBasicAuth(username, password string) http.Handler {
	return withBasicAuth(promhttp.Handler(), username, password)
}

func withBasicAuth(handler http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract credentials using http.BasicAuth
		reqUsername, reqPassword, ok := r.BasicAuth()
		if !ok || reqUsername != username || reqPassword != password {
			// Respond with a 401 Unauthorized if authentication fails
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// If authentication is successful, call the original handler
		handler.ServeHTTP(w, r)
	})
}
