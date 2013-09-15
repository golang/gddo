// Copyright 2012 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// +build ignore

// Command print fetches and prints package documentation.
//
// Usage: go run print.go importPath
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/gosrc"
)

var (
	etag  = flag.String("etag", "", "Etag")
	local = flag.Bool("local", false, "Get package from local directory.")
)

func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatal("Usage: go run print.go importPath")
	}
	path := flag.Args()[0]

	var (
		pdoc *doc.Package
		err  error
	)
	if *local {
		gosrc.SetLocalDevMode(os.Getenv("GOPATH"))
	}
	pdoc, err = doc.Get(http.DefaultClient, path, *etag)
	//}
	if err != nil {
		log.Fatal(err)
	}
	spew.Dump(pdoc)
}
