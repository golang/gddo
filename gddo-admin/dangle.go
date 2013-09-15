// Copyright 2013 Gary Burd
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

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/garyburd/gddo/database"
	"github.com/garyburd/gosrc"
)

var dangleCommand = &command{
	name:  "dangle",
	run:   dangle,
	usage: "dangle",
}

func dangle(c *command) {
	if len(c.flag.Args()) != 0 {
		c.printUsage()
		os.Exit(1)
	}
	db, err := database.New()
	if err != nil {
		log.Fatal(err)
	}
	m := make(map[string]int)
	err = db.Do(func(pi *database.PackageInfo) error {
		m[pi.PDoc.ImportPath] |= 1
		for _, p := range pi.PDoc.Imports {
			if gosrc.IsValidPath(p) {
				m[p] |= 2
			}
		}
		for _, p := range pi.PDoc.TestImports {
			if gosrc.IsValidPath(p) {
				m[p] |= 2
			}
		}
		for _, p := range pi.PDoc.XTestImports {
			if gosrc.IsValidPath(p) {
				m[p] |= 2
			}
		}
		return nil
	})

	for p, v := range m {
		if v == 2 {
			fmt.Println(p)
		}
	}
}
