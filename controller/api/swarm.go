package api

import (
	"bytes"
	log "github.com/Sirupsen/logrus"
	"net/http"
	"net/url"
)

func (a *Api) swarmRedirect(w http.ResponseWriter, req *http.Request) {
	var err error
	req.URL, err = url.ParseRequestURI(a.dUrl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info("begin to redirect ", req.URL, req.RequestURI)
	a.fwd.ServeHTTP(w, req)
}

type proxyWriter struct {
	Body       *bytes.Buffer
	Headers    *map[string][]string
	StatusCode *int
}

func (p proxyWriter) Header() http.Header {
	return *p.Headers
}
func (p proxyWriter) Write(data []byte) (int, error) {
	return p.Body.Write(data)
}
func (p proxyWriter) WriteHeader(code int) {
	*p.StatusCode = code
}
