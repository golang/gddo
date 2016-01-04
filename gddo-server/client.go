// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// This file implements an http.Client with request timeouts set by command
// line flags.

package main

import (
	"flag"
	"net"
	"net/http"
	"time"

	"github.com/golang/gddo/httputil"
)

var (
	dialTimeout    = flag.Duration("dial_timeout", 5*time.Second, "Timeout for dialing an HTTP connection.")
	requestTimeout = flag.Duration("request_timeout", 20*time.Second, "Time out for roundtripping an HTTP request.")
)

func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: httputil.NewAuthTransport(
			&http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&net.Dialer{
					Timeout:   *dialTimeout,
					KeepAlive: *requestTimeout / 2,
				}).Dial,
				ResponseHeaderTimeout: *requestTimeout / 2,
				TLSHandshakeTimeout:   *requestTimeout / 2,
			},
		),
		Timeout: *requestTimeout,
	}
}
