// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/golang/gddo/doc"
	"github.com/golang/gddo/gosrc"
)

var testdataPat = regexp.MustCompile(`/testdata(?:/|$)`)

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
	if strings.HasPrefix(importPath, "code.google.com/p/go.") {
		// Old import path for Go sub-repository.
		pdoc = nil
		err = gosrc.NotFoundError{Message: "old Go sub-repo", Redirect: "golang.org/x/" + importPath[len("code.google.com/p/go."):]}
	} else if blocked, e := db.IsBlocked(importPath); blocked && e == nil {
		pdoc = nil
		err = gosrc.NotFoundError{Message: "blocked."}
	} else if testdataPat.MatchString(importPath) {
		pdoc = nil
		err = gosrc.NotFoundError{Message: "testdata."}
	} else {
		var pdocNew *doc.Package
		pdocNew, err = doc.Get(httpClient, importPath, etag)
		message = append(message, "fetch:", int64(time.Since(start)/time.Millisecond))
		if err == nil && pdocNew.Name == "" && !hasSubdirs {
			for _, e := range pdocNew.Errors {
				message = append(message, "err:", e)
			}
			pdoc = nil
			err = gosrc.NotFoundError{Message: "no Go files or subdirs"}
		} else if _, ok := err.(gosrc.NotModifiedError); !ok {
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

	if err == nil {
		message = append(message, "put:", pdoc.Etag)
		if err := db.Put(pdoc, nextCrawl, false); err != nil {
			log.Printf("ERROR db.Put(%q): %v", importPath, err)
		}
		return pdoc, nil
	} else if e, ok := err.(gosrc.NotModifiedError); ok {
		if !pdoc.IsCmd && isInactivePkg(importPath, e.Since) {
			message = append(message, "delete inactive")
			if err := db.Delete(importPath); err != nil {
				log.Printf("ERROR db.Delete(%q): %v", importPath, err)
			}
			return nil, e
		}
		// Touch the package without updating and move on to next one.
		message = append(message, "touch")
		if err := db.SetNextCrawlEtag(pdoc.ProjectRoot, pdoc.Etag, nextCrawl); err != nil {
			log.Printf("ERROR db.SetNextCrawlEtag(%q): %v", importPath, err)
		}
		return pdoc, nil
	} else if err == gosrc.ErrQuickFork {
		message = append(message, "delete", err)
		if err := db.Delete(importPath); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", importPath, err)
		}
		return nil, err
	} else if e, ok := err.(gosrc.NotFoundError); ok {
		message = append(message, "notfound:", e)
		if err := db.Delete(importPath); err != nil {
			log.Printf("ERROR db.Delete(%q): %v", importPath, err)
		}
		return nil, e
	} else {
		message = append(message, "ERROR:", err)
		return nil, err
	}
}

// isInactivePkg reports whether the specified package is not imported
// and has not been modified in 2 years.
func isInactivePkg(pkg string, lastCommitted time.Time) bool {
	if lastCommitted.IsZero() ||
		time.Now().Before(lastCommitted.Add(2*365*24*time.Hour)) {
		return false
	}
	n, err := db.ImporterCount(pkg)
	if err != nil {
		log.Printf("ERROR db.ImporterCount(%q): %v", pkg, err)
	}
	return n == 0
}
