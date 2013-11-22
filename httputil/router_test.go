// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package httputil_test

import (
	"github.com/garyburd/gddo/httputil"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

type routeTestHandler string

func (h routeTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := httputil.RouteVars(r)
	var keys []string
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	w.Write([]byte(string(h)))
	for _, key := range keys {
		w.Write([]byte(" "))
		w.Write([]byte(key))
		w.Write([]byte(":"))
		w.Write([]byte(vars[key]))
	}
}

var routeTests = []struct {
	url    string
	method string
	status int
	body   string
}{
	{url: "/Bogus/Path", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/Bogus/Path", method: "POST", status: http.StatusNotFound, body: ""},
	{url: "/", method: "GET", status: http.StatusOK, body: "home-get"},
	{url: "/", method: "HEAD", status: http.StatusOK, body: "home-get"},
	{url: "/", method: "POST", status: http.StatusMethodNotAllowed, body: ""},
	{url: "/a", method: "GET", status: http.StatusOK, body: "a-get"},
	{url: "/a", method: "HEAD", status: http.StatusOK, body: "a-get"},
	{url: "/a", method: "POST", status: http.StatusOK, body: "a-*"},
	{url: "/a/", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/b", method: "GET", status: http.StatusOK, body: "b-get"},
	{url: "/b", method: "HEAD", status: http.StatusOK, body: "b-get"},
	{url: "/b", method: "POST", status: http.StatusOK, body: "b-post"},
	{url: "/b", method: "PUT", status: http.StatusMethodNotAllowed, body: ""},
	{url: "/c", method: "GET", status: http.StatusOK, body: "c-*"},
	{url: "/c", method: "HEAD", status: http.StatusOK, body: "c-*"},
	{url: "/d", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/d/", method: "GET", status: http.StatusOK, body: "d"},
	{url: "/e", method: "GET", status: http.StatusOK, body: "e"},
	{url: "/e/", method: "GET", status: http.StatusOK, body: "e-slash"},
	{url: "/f/foo", method: "GET", status: http.StatusOK, body: "f x:foo"},
	{url: "/f/foo/", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/g/foo/bar", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/g/foo/bar/", method: "GET", status: http.StatusOK, body: "g x:foo y:bar"},
	{url: "/h/foo", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/h/99", method: "GET", status: http.StatusOK, body: "h x:99"},
	{url: "/h/xx/i", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/h/xx/i/", method: "GET", status: http.StatusOK, body: "i"},
	{url: "/j/foo/d", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/j/foo/d/", method: "GET", status: http.StatusOK, body: "j x:foo"},
}

func TestRouter(t *testing.T) {
	router := httputil.NewRouter()
	router.Add("/").Get(routeTestHandler("home-get"))
	router.Add("/a").Get(routeTestHandler("a-get")).Method("*", routeTestHandler("a-*"))
	router.Add("/b").Get(routeTestHandler("b-get")).Post(routeTestHandler("b-post"))
	router.Add("/c").Method("*", routeTestHandler("c-*"))
	router.Add("/d/").Get(routeTestHandler("d"))
	router.Add("/e").Get(routeTestHandler("e"))
	router.Add("/e/").Get(routeTestHandler("e-slash"))
	router.Add("/f/<x>").Get(routeTestHandler("f"))
	router.Add("/f/").Get(routeTestHandler("f"))
	router.Add("/g/<x>/<y>/").Get(routeTestHandler("g"))
	router.Add("/h/<x:[0-9]+>").Get(routeTestHandler("h"))
	router.Add("/h/xx/i/").Get(routeTestHandler("i"))
	router.Add("/j/<x>/d/").Get(routeTestHandler("j"))

	for _, rt := range routeTests {
		r := &http.Request{URL: mustParseURL(rt.url), Method: rt.method}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != rt.status {
			t.Errorf("url=%s method=%s, status=%d, want %d", rt.url, rt.method, w.Code, rt.status)
		}
		if w.Code == http.StatusOK {
			if w.Body.String() != rt.body {
				t.Errorf("url=%s method=%s body=%q, want %q", rt.url, rt.method, w.Body.String(), rt.body)
			}
		}
	}
}

var hostRouterTests = []struct {
	host   string
	status int
	body   string
}{
	{host: "www.example.com", status: http.StatusOK, body: "www.example.com"},
	{host: "www.example.com:8080", status: http.StatusOK, body: "www.example.com"},
	{host: "foo.example.com", status: http.StatusOK, body: "*.example.com x:foo"},
	{host: "example.com", status: http.StatusOK, body: "default"},
}

func TestHostRouter(t *testing.T) {
	router := httputil.NewHostRouter()
	router.Add("www.example.com", routeTestHandler("www.example.com"))
	router.Add("<x>.example.com", routeTestHandler("*.example.com"))
	router.Add("<:.*>", routeTestHandler("default"))

	for _, tt := range hostRouterTests {
		r := &http.Request{Host: tt.host}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Errorf("host=%s, status=%d, want %d", tt.host, w.Code, tt.status)
		}
		if w.Code == http.StatusOK {
			if w.Body.String() != tt.body {
				t.Errorf("host=%s, body=%s, want %s", tt.host, w.Body.String(), tt.body)
			}
		}
	}
}
