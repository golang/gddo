// Copyright 2017 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// This file implements an http.Client with request timeouts set by command
// line flags.

package main

import (
	"net"
	"net/http"

	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/memcache"
	"github.com/spf13/viper"

	"github.com/golang/gddo/httputil"
)

func newHTTPClient() *http.Client {
	requestTimeout := viper.GetDuration(ConfigRequestTimeout)
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   viper.GetDuration(ConfigDialTimeout),
			KeepAlive: requestTimeout / 2,
		}).Dial,
		ResponseHeaderTimeout: requestTimeout / 2,
		TLSHandshakeTimeout:   requestTimeout / 2,
	}

	var rt http.RoundTripper
	if addr := viper.GetString(ConfigMemcacheAddr); addr != "" {
		ct := httpcache.NewTransport(memcache.New(addr))
		ct.Transport = t
		rt = httputil.NewAuthTransport(ct)
	} else {
		rt = httputil.NewAuthTransport(t)
	}
	return &http.Client{
		// Wrap the cached transport with GitHub authentication.
		Transport: rt,
		Timeout:   requestTimeout,
	}
}
