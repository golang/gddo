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

package main

import (
	"log"
	"os"
	"time"

	"github.com/garyburd/gddo/database"
	"github.com/garyburd/gddo/doc"
)

var reindexCommand = &command{
	name:  "reindex",
	run:   reindex,
	usage: "reindex",
}

func fix(pdoc *doc.Package) {
	/*
	   	for _, v := range pdoc.Consts {
	   	}
	   	for _, v := range pdoc.Vars {
	   	}
	   	for _, v := range pdoc.Funcs {
	   	}
	   	for _, t := range pdoc.Types {
	   		for _, v := range t.Consts {
	   		}
	   		for _, v := range t.Vars {
	   		}
	   		for _, v := range t.Funcs {
	   		}
	   		for _, v := range t.Methods {
	   		}
	   	}
	       for _, notes := range pdoc.Notes {
	           for _, v := range notes {
	           }
	       }
	*/
}

func reindex(c *command) {
	if len(c.flag.Args()) != 0 {
		c.printUsage()
		os.Exit(1)
	}
	db, err := database.New()
	if err != nil {
		log.Fatal(err)
	}
	var n int
	err = db.Do(func(pi *database.PackageInfo) error {
		n += 1
		fix(pi.PDoc)
		return db.Put(pi.PDoc, time.Time{})
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Updated %d documents", n)
}
