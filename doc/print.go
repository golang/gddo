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
	"fmt"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/garyburd/gddo/doc"
)

var (
	etag    = flag.String("etag", "", "Etag")
	local   = flag.Bool("local", false, "Get package from local directory.")
	present = flag.Bool("present", false, "Get presentation.")
)

func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatal("Usage: go run print.go importPath")
	}
	if *present {
		printPresentation(flag.Args()[0])
	} else {
		printPackage(flag.Args()[0])
	}
}

func printPackage(path string) {
	var (
		pdoc *doc.Package
		err  error
	)
	if *local {
		pdoc, err = doc.GetLocal(path, "", "", "%s", "#L%d")
	} else {
		pdoc, err = doc.Get(http.DefaultClient, path, *etag)
	}
	if err != nil {
		log.Fatal(err)
	}
	spew.Dump(pdoc)
}

func printPresentation(path string) {
	pres, err := doc.GetPresentation(http.DefaultClient, path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", pres.Files[pres.Filename])
	for name, data := range pres.Files {
		if name != pres.Filename {
			fmt.Printf("---------- %s ----------\n%s\n", name, data)
		}
	}
}
