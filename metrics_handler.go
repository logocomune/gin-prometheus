package ginprom

import (
	"crypto/subtle"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

// handlerConfig holds optional credentials for Basic Authentication on the
// metrics endpoint.
type handlerConfig struct {
	username string
	password string
}

// HandlerOption is a functional option that configures the metrics HTTP
// handler returned by [GetMetricHandler].
type HandlerOption func(*handlerConfig)

// WithBasicAuth protects the metrics endpoint with HTTP Basic Authentication.
// Requests that do not supply matching credentials receive a 401 Unauthorized
// response with a WWW-Authenticate challenge header.
//
// Example:
//
//	r.GET("/metrics", gin.WrapH(ginprom.GetMetricHandler(
//	    ginprom.WithBasicAuth("prometheus", "s3cr3t"),
//	)))
func WithBasicAuth(username, password string) HandlerOption {
	return func(c *handlerConfig) {
		c.username = username
		c.password = password
	}
}

// GetMetricHandler returns an [http.Handler] that serves the default
// Prometheus metrics page (equivalent to promhttp.Handler).  Pass
// [WithBasicAuth] to require authentication before metrics are exposed.
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

func withBasicAuth(handler http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract credentials using http.BasicAuth
		reqUsername, reqPassword, ok := r.BasicAuth()

		// Compare in constant time so response timing can't be used to guess
		// credentials byte-by-byte. subtle.ConstantTimeCompare requires equal
		// length inputs to stay constant-time, so mismatched lengths are
		// treated as a mismatch up front without leaking length info via a
		// short-circuited == check.
		usernameMatch := subtle.ConstantTimeCompare([]byte(reqUsername), []byte(username)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(reqPassword), []byte(password)) == 1

		if !ok || !usernameMatch || !passwordMatch {
			// Respond with a 401 Unauthorized if authentication fails
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// If authentication is successful, call the original handler
		handler.ServeHTTP(w, r)
	})
}
