package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/influxdb/influxdb"
	"github.com/influxdb/influxdb/httpd"
	"github.com/influxdb/influxdb/messaging"
	"github.com/influxdb/influxdb/raft"
)

// Handler represents an HTTP handler for InfluxDB node.
// Depending on its role, it will serve many different endpoints.
type Handler struct {
	Log    *raft.Log
	Broker *influxdb.Broker
	Server *influxdb.Server
	Config *Config
}

// NewHandler returns a new instance of Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP responds to HTTP request to the handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/raft") {
		h.serveRaft(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/messaging") {
		h.serveMessaging(w, r)
		return
	}

	h.serveData(w, r)
}

// serveMessaging responds to broker requests
func (h *Handler) serveMessaging(w http.ResponseWriter, r *http.Request) {
	// If we're running a broker, handle the broker endpoints
	if h.Broker != nil {
		mh := &messaging.Handler{
			Broker:      h.Broker.Broker,
			RaftHandler: &raft.Handler{Log: h.Log},
		}
		mh.ServeHTTP(w, r)
		return
	}

	if h.Server == nil {
		log.Println("no broker or server configured to handle messaging endpoints")
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		return
	}

	// Redirect to a valid broker to handle the request
	h.redirect(h.Server.BrokerURLs(), w, r)
}

// serveRaft responds to raft requests.
func (h *Handler) serveRaft(w http.ResponseWriter, r *http.Request) {
	if h.Log != nil {
		rh := raft.Handler{Log: h.Log}
		rh.ServeHTTP(w, r)
		return
	}

	if h.Server == nil {
		log.Println("no broker or server configured to handle messaging endpoints")
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		return
	}

	// Redirect to a valid broker to handle the request
	h.redirect(h.Server.BrokerURLs(), w, r)
}

// serveData responds to data requests
func (h *Handler) serveData(w http.ResponseWriter, r *http.Request) {
	if h.Server != nil {
		sh := httpd.NewHandler(h.Server, h.Config.Authentication.Enabled, version)
		sh.WriteTrace = h.Config.Logging.WriteTracing
		sh.ServeHTTP(w, r)
		return
	}

	if h.Broker == nil {
		log.Println("no broker or server configured to handle messaging endpoints")
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		return

	}

	// Redirect to a valid data URL to handle the request
	h.redirect(h.Broker.Topic(influxdb.BroadcastTopicID).DataURLs(), w, r)
}

// redirect redirects a request to URL in u.  If u is an empty slice,
// a 503 is returned
func (h *Handler) redirect(u []url.URL, w http.ResponseWriter, r *http.Request) {
	// No valid URLs, return an error
	if len(u) == 0 {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	// TODO: Log to internal stats to track how frequently this is happening. If
	// this is happening frequently, the clients are using a suboptimal endpoint

	// Redirect the client to a valid data node that can handle the request
	http.Redirect(w, r, u[0].String()+r.RequestURI, http.StatusTemporaryRedirect)
}
