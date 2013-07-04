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
	"github.com/garyburd/gddo/database"
	"github.com/garyburd/gddo/doc"
	"log"
	"os"
	"strings"
)

func jaccardIndex(a, b *doc.Package) float64 {
	set := make(map[string]int)
	for i, pdoc := range []*doc.Package{a, b} {
		mask := 1 << uint(i)
		for _, f := range pdoc.Files {
			set[f.Name] |= mask
		}
		for _, p := range pdoc.Imports {
			set[p] |= mask
		}
		for _, p := range pdoc.TestImports {
			set[p] |= mask
		}
		for _, f := range pdoc.Funcs {
			set[f.Name] |= mask
		}
		for _, t := range pdoc.Types {
			set[t.Name] |= mask
			for _, f := range t.Funcs {
				set[f.Name] |= mask
			}
			for _, f := range t.Methods {
				set[f.Recv+"."+f.Name] |= mask
			}
		}
	}
	n := 0
	for _, bits := range set {
		if bits == 3 {
			n += 1
		}
	}
	return float64(n) / float64(len(set))
}

var (
	pruneCommand = &command{
		name:  "prune",
		usage: "prune",
	}
	pruneDryRun = pruneCommand.flag.Bool("n", false, "Dry run.")
)

func init() {
	pruneCommand.run = prune
}

func prune(c *command) {
	if len(c.flag.Args()) != 0 {
		c.printUsage()
		os.Exit(1)
	}
	db, err := database.New()
	if err != nil {
		log.Fatal(err)
	}

	paths := make(map[string]bool)

	err = db.Do(func(pi *database.PackageInfo) error {
		pdoc := pi.PDoc
		if pdoc.ProjectRoot == "" {
			return nil
		}

		i := strings.LastIndex(pdoc.ImportPath, "/")
		if i < 0 {
			return nil
		}
		suffix := pdoc.ImportPath[i:]

		imports := make(map[string]bool)
		for _, p := range pdoc.Imports {
			imports[p] = true
		}

		pathLists := [][]string{pdoc.TestImports, pdoc.XTestImports, pdoc.References}
		if pdoc.ProjectRoot != pdoc.ImportPath {
			if pdocRoot, _, _ := db.GetDoc(pdoc.ProjectRoot); pdocRoot != nil {
				pathLists = append(pathLists, pdocRoot.References)
			}
		}

		fork := ""

	forkCheck:
		for _, list := range pathLists {
			for _, p := range list {
				if p != pdoc.ImportPath && strings.HasSuffix(p, suffix) && !imports[p] {
					pdocTest, _, _ := db.GetDoc(p)
					if pdocTest != nil && pdocTest.Name == pdoc.Name && jaccardIndex(pdocTest, pdoc) > 0.75 {
						fork = pdocTest.ImportPath
						break forkCheck
					}
				}
			}
		}

		if fork != "" {
			log.Printf("%s is fork of %s", pdoc.ImportPath, fork)
			if !*pruneDryRun {
				for _, pkg := range pi.Pkgs {
					if err := db.Delete(pkg.Path); err != nil {
						log.Printf("Error deleting %s, %v", pkg.Path, err)
					}
				}
				if err := db.Delete(pdoc.ImportPath); err != nil {
					log.Printf("Error deleting %s, %v", pdoc.ImportPath, err)
				}
			}
		} else {
			keep := pi.Score > 0
			if pdoc.IsCmd && pdoc.Synopsis != "" && len(pdoc.Doc) > len(pdoc.Synopsis) {
				// Keep a command if there's actually some documentation.
				keep = true
			}
			p := pdoc.ImportPath
			for {
				paths[p] = paths[p] || keep
				if len(p) <= len(pdoc.ProjectRoot) {
					break
				} else if i := strings.LastIndex(p, "/"); i < 0 {
					break
				} else {
					p = p[:i]
				}
			}
		}
		return nil
	})

	for p, keep := range paths {
		if !keep {
			log.Printf("%s has rank 0", p)
			if !*pruneDryRun {
				if err := db.Delete(p); err != nil {
					log.Printf("Error deleting %s, %v", p, err)
				}
			}
		}
	}

	if err != nil {
		log.Fatal(err)
	}
}
