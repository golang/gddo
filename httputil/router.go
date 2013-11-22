// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package httputil

import (
	"bytes"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
)

type Error func(w http.ResponseWriter, r *http.Request, status int, err error)

var (
	varsMu sync.Mutex
	vars   = make(map[*http.Request]map[string]string)
)

// RouteVars returns the matched pattern variables for the request.
func RouteVars(r *http.Request) map[string]string {
	varsMu.Lock()
	m := vars[r]
	varsMu.Unlock()
	return m
}

func addRouteVars(r *http.Request, names, values []string) bool {
	varsMu.Lock()
	m, found := vars[r]
	if !found {
		m = make(map[string]string)
		vars[r] = m
	}
	varsMu.Unlock()
	for i, name := range names {
		m[name] = values[i]
	}
	return !found
}

func clearRouteVars(r *http.Request) {
	varsMu.Lock()
	delete(vars, r)
	varsMu.Unlock()
}

// Router is a request handler that dispatches HTTP requests to other handlers
// using the request URL path and the request method.
//
// A router has a list of routes. A route is a request path pattern and a
// collection of (method, handler) pairs.
//
// A path pattern is a string with embedded parameters. A parameter has the
// syntax:
//
//  '<' name (':' regular-expression)? '>'
//
// If the regular expression is not specified, then the regular expression
// [^/]+ is used.
//
// The pattern must begin with the character '/'.
//
// A router dispatches requests by matching the request URL path against the
// route patterns in the order that the routes were added. If a matching route
// is not found, then the router responds to the request with HTTP status 404.
//
// If a matching route is found, then the router looks for a handler using the
// request method, "GET" if the request method is "HEAD" and "*". If a handler
// is not found, then the router responds to the request with HTTP status 405.
//
// Call the RouteVars function to get the matched parameter values for a
// request.
//
// If a pattern ends with '/', then the router redirects the URL without the
// trailing slash to the URL with the trailing slash.
type Router struct {
	h404        http.Handler
	h405        http.Handler
	simpleMatch map[string]*Route
	routes      []*Route
}

type Route struct {
	pattern  string
	addSlash bool
	regexp   *regexp.Regexp
	names    []string
	handlers map[string]http.Handler
}

var parameterRegexp = regexp.MustCompile("<([A-Za-z0-9_]*)(:[^>]*)?>")

// compilePattern compiles the pattern to a regular expression and array of
// parameter names.
func compilePattern(pattern string, addSlash bool, sep string) (*regexp.Regexp, []string) {
	var buf bytes.Buffer
	var names []string
	buf.WriteByte('^')
	for {
		a := parameterRegexp.FindStringSubmatchIndex(pattern)
		if len(a) == 0 {
			buf.WriteString(regexp.QuoteMeta(pattern))
			break
		} else {
			buf.WriteString(regexp.QuoteMeta(pattern[0:a[0]]))
			name := pattern[a[2]:a[3]]
			if name != "" {
				names = append(names, pattern[a[2]:a[3]])
				buf.WriteString("(")
			}
			if a[4] >= 0 {
				buf.WriteString(pattern[a[4]+1 : a[5]])
			} else {
				buf.WriteString("[^" + sep + "]+")
			}
			if name != "" {
				buf.WriteString(")")
			}
			pattern = pattern[a[1]:]
		}
	}
	if addSlash {
		buf.WriteString("?")
	}
	buf.WriteString("$")
	return regexp.MustCompile(buf.String()), names
}

// Add adds a new route for the specified pattern.
func (router *Router) Add(pattern string) *Route {
	if pattern == "" || pattern[0] != '/' {
		panic("httputil: invalid route pattern " + pattern)
	}
	route := &Route{pattern: pattern}
	route.handlers = make(map[string]http.Handler)
	route.addSlash = pattern != "/" && pattern[len(pattern)-1] == '/'
	route.regexp, route.names = compilePattern(pattern, route.addSlash, "/")
	if len(route.names) > 0 {
		router.routes = append(router.routes, route)
	} else {
		if foundRoute, _ := router.findRoute(pattern); foundRoute != nil {
			panic("httputil: pattern " + pattern + " matches route " + foundRoute.pattern)
		}
		router.simpleMatch[pattern] = route
		if route.addSlash {
			pattern = pattern[:len(pattern)-1]
			if foundRoute, _ := router.findRoute(pattern); foundRoute == nil {
				router.simpleMatch[pattern] = route
			}
		}
	}
	return route
}

// Method sets the handler for the given HTTP request method. Use "*" to match
// all methods.
func (route *Route) Method(method string, handler http.Handler) *Route {
	route.handlers[method] = handler
	return route
}

// Get adds a "GET" handler to the route.
func (route *Route) Get(handler http.Handler) *Route {
	return route.Method("GET", handler)
}

// Post adds a "POST" handler to the route.
func (route *Route) Post(handler http.Handler) *Route {
	return route.Method("POST", handler)
}

// GetFunc adds a "GET" handler to the route.
func (route *Route) GetFunc(handler func(http.ResponseWriter, *http.Request)) *Route {
	return route.Method("GET", http.HandlerFunc(handler))
}

// PostFunc adds a "POST" handler to the route.
func (route *Route) PostFunc(handler func(http.ResponseWriter, *http.Request)) *Route {
	return route.Method("POST", http.HandlerFunc(handler))
}

// addSlash redirects to the request URL with a trailing slash.
func addSlash(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path + "/"
	if len(r.URL.RawQuery) > 0 {
		path = path + "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, path, 301)
}

func (router *Router) findRoute(path string) (*Route, []string) {
	if r, ok := router.simpleMatch[path]; ok {
		return r, nil
	}
	for _, r := range router.routes {
		values := r.regexp.FindStringSubmatch(path)
		if values != nil {
			return r, values[1:]
		}
	}
	return nil, nil
}

// find the handler and path parameters using the path component of the request
// URL and the request method.
func (router *Router) findHandler(path, method string) (http.Handler, []string, []string) {
	route, values := router.findRoute(path)
	if route == nil {
		return router.h404, nil, nil
	}
	if route.addSlash && path[len(path)-1] != '/' {
		return http.HandlerFunc(addSlash), nil, nil
	}
	h := route.handlers[method]
	if h == nil && method == "HEAD" {
		h = route.handlers["GET"]
	}
	if h == nil {
		h = route.handlers["*"]
	}
	if h == nil {
		return router.h405, nil, nil
	}
	return h, route.names, values
}

func cleanURLPath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	slash := p[len(p)-1] == '/'
	p = path.Clean(p)
	if slash {
		p += "/"
	}
	return p
}

// ServerHTTP dispatches the request to a registered handler.
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := cleanURLPath(r.URL.Path)
	if p != r.URL.Path {
		http.Redirect(w, r, p, 301)
		return
	}
	h, names, values := router.findHandler(r.URL.Path, r.Method)
	if addRouteVars(r, names, values) {
		defer clearRouteVars(r)
	}
	h.ServeHTTP(w, r)
}

// Error sets the function used to generate error responses from the router.
// The default error function calls the net/http Error function.
func (router *Router) Error(errfn Error) {
	router.h404 = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { errfn(w, r, 404, nil) })
	router.h405 = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { errfn(w, r, 405, nil) })
}

// NewRouter allocates and initializes a new Router.
func NewRouter() *Router {
	router := &Router{simpleMatch: make(map[string]*Route)}
	router.Error(func(w http.ResponseWriter, r *http.Request, code int, err error) {
		http.Error(w, http.StatusText(code), code)
	})
	return router
}

// HostRouter is a request handler that dispatches HTTP requests to other
// handlers using the host header.
//
// A host router has a list of routes where each route is a (pattern, handler)
// pair. The router dispatches requests by matching the host header against
// the patterns in the order that the routes were registered. If a matching
// route is found, the request is dispatched to the route's handler.
//
// A pattern is a string with embedded parameters. A parameter has the syntax:
//
//  '<' name (':' regexp)? '>'
//
// If the regular expression is not specified, then the regular expression
// [^.]+ is used.
//
// Call the RouteVars function to get the matched parameter values for a
// request.
type HostRouter struct {
	routes []hostRoute
	errfn  Error
}

type hostRoute struct {
	regexp  *regexp.Regexp
	names   []string
	handler http.Handler
}

// NewHostRouter allocates and initializes a new HostRouter.
func NewHostRouter() *HostRouter {
	return &HostRouter{
		errfn: func(w http.ResponseWriter, r *http.Request, status int, err error) {
			http.Error(w, http.StatusText(status), status)
		},
	}
}

// Error sets the function used to generate error responses from the router.
// The default error function calls the net/http Error function.
func (router *HostRouter) Error(errfn Error) {
	router.errfn = errfn
}

// Add adds a handler for the given pattern.
func (router *HostRouter) Add(hostPattern string, handler http.Handler) {
	regex, names := compilePattern(hostPattern, false, ".")
	router.routes = append(router.routes, hostRoute{regexp: regex, names: names, handler: handler})
}

func (router *HostRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(StripPort(r.Host))
	for _, route := range router.routes {
		values := route.regexp.FindStringSubmatch(host)
		if values == nil {
			continue
		}
		if addRouteVars(r, route.names, values[1:]) {
			defer clearRouteVars(r)
		}
		route.handler.ServeHTTP(w, r)
		return
	}
	router.errfn(w, r, http.StatusNotFound, nil)
}

// StripPort removes the port specification from an address.
func StripPort(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return s
}
