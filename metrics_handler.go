package ginprom

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	// Fixed-length digests avoid the length-mismatch shortcut in ConstantTimeCompare.
	expectedUsername := sha256.Sum256([]byte(username))
	expectedPassword := sha256.Sum256([]byte(password))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqUsername, reqPassword, ok := r.BasicAuth()
		providedUsername := sha256.Sum256([]byte(reqUsername))
		providedPassword := sha256.Sum256([]byte(reqPassword))
		usernameMatches := subtle.ConstantTimeCompare(providedUsername[:], expectedUsername[:])
		passwordMatches := subtle.ConstantTimeCompare(providedPassword[:], expectedPassword[:])
		if !ok || usernameMatches&passwordMatches != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		handler.ServeHTTP(w, r)
	})
}
