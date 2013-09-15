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
	"regexp"
	"strings"
	"time"

	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/gosrc"
)

var nestedProjectPat = regexp.MustCompile(`/(?:github\.com|launchpad\.net|code\.google\.com/p|bitbucket\.org|labix\.org)/`)

func exists(path string) bool {
	b, err := db.Exists(path)
	if err != nil {
		b = false
	}
	return b
}

// crawlDoc fetches the package documentation from the VCS and updates the database.
func crawlDoc(source string, importPath string, pdoc *doc.Package, hasSubdirs bool, nextCrawl time.Time) (*doc.Package, error) {
	message := []interface{}{source}
	defer func() {
		message = append(message, importPath)
		log.Println(message...)
	}()

	if !nextCrawl.IsZero() {
		d := time.Since(nextCrawl) / time.Hour
		if d > 0 {
			message = append(message, "late:", int64(d))
		}
	}

	etag := ""
	if pdoc != nil {
		etag = pdoc.Etag
		message = append(message, "etag:", etag)
	}

	start := time.Now()
	var err error
	if i := strings.Index(importPath, "/src/pkg/"); i > 0 && gosrc.IsGoRepoPath(importPath[i+len("/src/pkg/"):]) {
		// Go source tree mirror.
		pdoc = nil
		err = gosrc.NotFoundError{Message: "Go source tree mirror."}
	} else if i := strings.Index(importPath, "/libgo/go/"); i > 0 && gosrc.IsGoRepoPath(importPath[i+len("/libgo/go/"):]) {
		// Go Frontend source tree mirror.
		pdoc = nil
		err = gosrc.NotFoundError{Message: "Go Frontend source tree mirror."}
	} else if m := nestedProjectPat.FindStringIndex(importPath); m != nil && exists(importPath[m[0]+1:]) {
		pdoc = nil
		err = gosrc.NotFoundError{Message: "Copy of other project."}
	} else if blocked, e := db.IsBlocked(importPath); blocked && e == nil {
		pdoc = nil
		err = gosrc.NotFoundError{Message: "Blocked."}
	} else {
		var pdocNew *doc.Package
		pdocNew, err = doc.Get(httpClient, importPath, etag)
		message = append(message, "fetch:", int64(time.Since(start)/time.Millisecond))
		if err == nil && pdocNew.Name == "" && !hasSubdirs {
			pdoc = nil
			err = gosrc.NotFoundError{Message: "No Go files or subdirs"}
		} else if err != gosrc.ErrNotModified {
			pdoc = pdocNew
		}
	}

	nextCrawl = start.Add(*maxAge)
	switch {
	case strings.HasPrefix(importPath, "github.com/") || (pdoc != nil && len(pdoc.Errors) > 0):
		nextCrawl = start.Add(*maxAge * 7)
	case strings.HasPrefix(importPath, "gist.github.com/"):
		// Don't spend time on gists. It's silly thing to do.
		nextCrawl = start.Add(*maxAge * 30)
	}

	switch {
	case err == nil:
		message = append(message, "put:", pdoc.Etag)
		if err := db.Put(pdoc, nextCrawl); err != nil {
			log.Printf("ERROR db.Put(%q): %v", importPath, err)
		}
	case err == gosrc.ErrNotModified:
		message = append(message, "touch")
		if err := db.SetNextCrawlEtag(pdoc.ProjectRoot, pdoc.Etag, nextCrawl); err != nil {
			log.Printf("ERROR db.SetNextCrawl(%q): %v", importPath, err)
		}
	case gosrc.IsNotFound(err):
		message = append(message, "notfound:", err)
		if err := db.Delete(importPath); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", importPath, err)
		}
	default:
		message = append(message, "ERROR:", err)
		return nil, err
	}

	return pdoc, nil
}
