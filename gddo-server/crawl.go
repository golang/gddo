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
func crawlDoc(source string, path string, pdoc *doc.Package, hasSubdirs bool, nextCrawl time.Time) (*doc.Package, error) {
	message := []interface{}{source}
	defer func() {
		message = append(message, path)
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
	if i := strings.Index(path, "/src/pkg/"); i > 0 && doc.IsGoRepoPath(path[i+len("/src/pkg/"):]) {
		// Go source tree mirror.
		pdoc = nil
		err = doc.NotFoundError{Message: "Go source tree mirror."}
	} else if i := strings.Index(path, "/libgo/go/"); i > 0 && doc.IsGoRepoPath(path[i+len("/libgo/go/"):]) {
		// Go Frontend source tree mirror.
		pdoc = nil
		err = doc.NotFoundError{Message: "Go Frontend source tree mirror."}
	} else if m := nestedProjectPat.FindStringIndex(path); m != nil && exists(path[m[0]+1:]) {
		pdoc = nil
		err = doc.NotFoundError{Message: "Copy of other project."}
	} else if blocked, e := db.IsBlocked(path); blocked && e == nil {
		pdoc = nil
		err = doc.NotFoundError{Message: "Blocked."}
	} else {
		var pdocNew *doc.Package
		pdocNew, err = doc.Get(httpClient, path, etag)
		message = append(message, "fetch:", int64(time.Since(start)/time.Millisecond))
		if err != doc.ErrNotModified {
			pdoc = pdocNew
		}
	}

	nextCrawl = start.Add(*maxAge)
	if strings.HasPrefix(path, "github.com/") || (pdoc != nil && len(pdoc.Errors) > 0) {
		nextCrawl = start.Add(*maxAge * 7)
	}

	switch {
	case err == nil:
		message = append(message, "put:", pdoc.Etag)
		if err := db.Put(pdoc, nextCrawl); err != nil {
			log.Printf("ERROR db.Put(%q): %v", path, err)
		}
	case err == doc.ErrNotModified:
		message = append(message, "touch")
		if err := db.SetNextCrawlEtag(pdoc.ProjectRoot, pdoc.Etag, nextCrawl); err != nil {
			log.Printf("ERROR db.SetNextCrawl(%q): %v", path, err)
		}
	case doc.IsNotFound(err):
		message = append(message, "notfound:", err)
		if err := db.Delete(path); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", path, err)
		}
	default:
		message = append(message, "ERROR:", err)
		return nil, err
	}

	return pdoc, nil
}
