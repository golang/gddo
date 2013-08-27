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
	"sort"

	"github.com/garyburd/gddo/database"
)

var statsCommand = &command{
	name:  "stats",
	run:   stats,
	usage: "stats",
}

type itemSize struct {
	path string
	size int
}

type bySizeDesc []itemSize

func (p bySizeDesc) Len() int           { return len(p) }
func (p bySizeDesc) Less(i, j int) bool { return p[i].size > p[j].size }
func (p bySizeDesc) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func stats(c *command) {
	if len(c.flag.Args()) != 0 {
		c.printUsage()
		os.Exit(1)
	}
	db, err := database.New()
	if err != nil {
		log.Fatal(err)
	}

	var packageSizes []itemSize
	projectSizes := make(map[string]int)
	err = db.Do(func(pi *database.PackageInfo) error {
		packageSizes = append(packageSizes, itemSize{pi.PDoc.ImportPath, pi.Size})
		projectSizes[pi.PDoc.ProjectRoot] += pi.Size
		return nil
	})

	var sizes []itemSize
	for path, size := range projectSizes {
		sizes = append(sizes, itemSize{path, size})
	}
	sort.Sort(bySizeDesc(sizes))
	for _, size := range sizes {
		fmt.Printf("%6d %s\n", size.size, size.path)
	}

	sort.Sort(bySizeDesc(packageSizes))
	for _, size := range packageSizes {
		fmt.Printf("%6d %s\n", size.size, size.path)
	}

}
