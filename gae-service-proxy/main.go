// Copyright 2020 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// Command gae-service-proxy serves as a proxy for requests to App Engine’s
// remote API endpoints.
package main

import (
	"google.golang.org/appengine"
	_ "google.golang.org/appengine/remote_api"
)

func main() {
	appengine.Main()
}
