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
	"log"
	"net"
	"net/http"
	"time"
)

var (
	dialTimeout    = flag.Duration("dial_timeout", 5*time.Second, "Timeout for dialing an HTTP connection.")
	requestTimeout = flag.Duration("request_timeout", 20*time.Second, "Time out for roundtripping an HTTP request.")
)

func timeoutDial(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, *dialTimeout)
}

type transport struct {
	t http.Transport
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	timer := time.AfterFunc(*requestTimeout, func() {
		t.t.CancelRequest(req)
		log.Printf("Canceled request for %s", req.URL)
	})
	defer timer.Stop()
	resp, err := t.t.RoundTrip(req)
	return resp, err
}

var httpTransport = &transport{t: http.Transport{Dial: timeoutDial, ResponseHeaderTimeout: *requestTimeout / 2}}
var httpClient = &http.Client{Transport: httpTransport}
