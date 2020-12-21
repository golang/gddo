// Copyright 2020 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/net/context/ctxhttp"
)

type pkggodevEvent struct {
	Host    string
	Path    string
	Status  int
	URL     string
	Latency time.Duration
	Error   string
	// If a request 404s, make a request to fetch it and store the response.
	FetchStatus   int
	FetchResponse string
}

func teeRequestToPkgGoDev(godocReq *http.Request, latency time.Duration, isRobot bool, status int) (gddoEvent *gddoEvent, pkgEvent *pkggodevEvent) {
	gddoEvent = newGDDOEvent(godocReq, latency, isRobot, status)
	u := pkgGoDevURL(godocReq.URL)

	// Strip the utm_source from the URL.
	vals := u.Query()
	vals.Del("utm_source")
	u.RawQuery = vals.Encode()

	pkgEvent = &pkggodevEvent{
		Host: u.Host,
		Path: u.Path,
		URL:  u.String(),
	}
	start := time.Now()
	status, errResp := makeRequest(godocReq.Context(), u.String())
	pkgEvent.Status = status
	pkgEvent.Latency = time.Since(start)
	// The response will always be an error here if not empty.
	pkgEvent.Error = errResp

	if pkgEvent.Status == http.StatusNotFound && gddoEvent.Status == http.StatusOK {
		// If the request was successful on godoc.org but returned a 404 on
		// pkg.go.dev make a fetch request.
		status, body := makeRequest(godocReq.Context(), "/fetch"+u.String())
		pkgEvent.FetchStatus = status
		pkgEvent.FetchResponse = body
	}
	return gddoEvent, pkgEvent
}

func makeRequest(ctx context.Context, url string) (int, string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("http.NewRequest: %v", err)
	}
	xfwd := req.Header.Get("X-Forwarded-for")
	req.Header.Set("X-Godoc-Forwarded-for", xfwd)
	resp, err := ctxhttp.Do(ctx, http.DefaultClient, req)
	if err != nil {
		// Use StatusBadGateway to indicate the upstream error.
		return http.StatusBadGateway, err.Error()
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Sprintf("can't read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

// doNotTeeURLsToPkgGoDev are paths that should not be teed to pkg.go.dev.
var doNotTeeURLsToPkgGoDev = map[string]bool{
	"/-/bot":     true,
	"/-/refresh": true,
}

// doNotTeeExtsToPkgGoDev are URL extensions that should not be teed to
// pkg.go.dev.
var doNotTeeExtsToPkgGoDev = map[string]bool{
	".css":  true,
	".html": true,
	".js":   true,
	".txt":  true,
	".xml":  true,
	".ico":  true,
}

// shouldTeeRequest reports whether a request should be teed to pkg.go.dev.
func shouldTeeRequest(u string) bool {
	// Don't tee App Engine requests to pkg.go.dev.
	if strings.HasPrefix(u, "/_ah/") {
		return false
	}
	ext := filepath.Ext(u)
	if doNotTeeExtsToPkgGoDev[ext] {
		return false
	}
	if doNotTeeURLsToPkgGoDev[u] {
		return false
	}
	return true
}

type gddoEvent struct {
	Host        string
	Path        string
	Status      int
	URL         string
	Header      http.Header
	Latency     time.Duration
	IsRobot     bool
	UsePkgGoDev bool
	Error       error
}

func newGDDOEvent(r *http.Request, latency time.Duration, isRobot bool, status int) *gddoEvent {
	targetURL := url.URL{
		Scheme:   "https",
		Host:     r.URL.Host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	if targetURL.Host == "" && r.Host != "" {
		targetURL.Host = r.Host
	}
	return &gddoEvent{
		Host:        targetURL.Host,
		Path:        r.URL.Path,
		Status:      status,
		URL:         targetURL.String(),
		Header:      r.Header,
		Latency:     latency,
		IsRobot:     isRobot,
		UsePkgGoDev: shouldRedirectToPkgGoDev(r),
	}
}

func userReturningFromPkgGoDev(req *http.Request) bool {
	return req.FormValue("utm_source") == "backtogodoc"
}

const (
	pkgGoDevRedirectCookie = "pkggodev-redirect"
	pkgGoDevRedirectParam  = "redirect"
	pkgGoDevRedirectOn     = "on"
	pkgGoDevRedirectOff    = "off"
	pkgGoDevHost           = "pkg.go.dev"
)

func shouldRedirectToPkgGoDev(req *http.Request) bool {
	// API requests are not redirected.
	if strings.HasPrefix(req.URL.Host, "api") {
		return false
	}
	redirectParam := req.FormValue(pkgGoDevRedirectParam)
	if redirectParam == pkgGoDevRedirectOn || redirectParam == pkgGoDevRedirectOff {
		return redirectParam == pkgGoDevRedirectOn
	}
	cookie, err := req.Cookie(pkgGoDevRedirectCookie)
	return (err == nil && cookie.Value == pkgGoDevRedirectOn)
}

// pkgGoDevRedirectHandler redirects requests from godoc.org to pkg.go.dev,
// based on whether a cookie is set for pkggodev-redirect. The cookie
// can be turned on/off using a query param.
func pkgGoDevRedirectHandler(f func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userReturningFromPkgGoDev(r) {
			return f(w, r)
		}

		redirectParam := r.FormValue(pkgGoDevRedirectParam)

		if redirectParam == pkgGoDevRedirectOn {
			cookie := &http.Cookie{Name: pkgGoDevRedirectCookie, Value: redirectParam, Path: "/"}
			http.SetCookie(w, cookie)
		}
		if redirectParam == pkgGoDevRedirectOff {
			cookie := &http.Cookie{Name: pkgGoDevRedirectCookie, Value: "", MaxAge: -1, Path: "/"}
			http.SetCookie(w, cookie)
		}

		if !shouldRedirectToPkgGoDev(r) {
			return f(w, r)
		}

		http.Redirect(w, r, pkgGoDevURL(r.URL).String(), http.StatusFound)
		return nil
	}
}

const goGithubRepoURLPath = "/github.com/golang/go"

func pkgGoDevURL(godocURL *url.URL) *url.URL {
	u := &url.URL{Scheme: "https", Host: pkgGoDevHost}
	q := url.Values{"utm_source": []string{"godoc"}}

	if strings.Contains(godocURL.Path, "/vendor/") || strings.HasSuffix(godocURL.Path, "/vendor") {
		u.Path = "/"
		u.RawQuery = q.Encode()
		return u
	}

	if strings.HasPrefix(godocURL.Path, goGithubRepoURLPath) ||
		strings.HasPrefix(godocURL.Path, goGithubRepoURLPath+"/src") {
		u.Path = strings.TrimPrefix(strings.TrimPrefix(godocURL.Path, goGithubRepoURLPath), "/src")
		if u.Path == "" {
			u.Path = "/std"
		}
		u.RawQuery = q.Encode()
		return u
	}

	_, isSVG := godocURL.Query()["status.svg"]
	_, isPNG := godocURL.Query()["status.png"]
	if isSVG || isPNG {
		u.Path = "/badge" + godocURL.Path
		u.RawQuery = q.Encode()
		return u
	}

	switch godocURL.Path {
	case "/-/go":
		u.Path = "/std"
	case "/-/about":
		u.Path = "/about"
	case "/C":
		u.Path = "/C"
	case "/":
		if qparam := godocURL.Query().Get("q"); qparam != "" {
			u.Path = "/search"
			q.Set("q", qparam)
		} else {
			u.Path = "/"
		}
	case "":
		u.Path = ""
	case "/-/subrepo":
		u.Path = "/search"
		q.Set("q", "golang.org/x")
	default:
		{
			// If the import path is invalid, redirect to
			// https://golang.org/issue/43036, so that the users has more context
			// on why this path does not work on pkg.go.dev.
			if err := module.CheckImportPath(strings.TrimPrefix(godocURL.Path, "/")); err != nil {
				u.Host = "golang.org"
				u.Path = "/issue/43036"
				return u
			}

			u.Path = godocURL.Path
			if _, ok := godocURL.Query()["imports"]; ok {
				q.Set("tab", "imports")
			} else if _, ok := godocURL.Query()["importers"]; ok {
				q.Set("tab", "importedby")
			}
		}
	}

	u.RawQuery = q.Encode()
	return u
}
