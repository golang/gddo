package main

import "net/http"

type httpsEnforcerHandler struct {
	h http.Handler
}

func (h httpsEnforcerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host == "godoc.org" {
		w.Header().Add("Strict-Transport-Security", "max-age=631138519; includeSubdomains; preload")
		if r.Header.Get("X-Scheme") != "https" {
			r.URL.Scheme = "https"
			http.Redirect(w, r, r.URL.String(), http.StatusFound)
			return
		}
	}
	h.h.ServeHTTP(w, r)
}
