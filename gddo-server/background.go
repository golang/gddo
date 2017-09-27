// Copyright 2017 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"log"
	"time"

	"github.com/spf13/viper"

	"github.com/golang/gddo/gosrc"
)

func doCrawl() error {
	// Look for new package to crawl.
	importPath, hasSubdirs, err := db.PopNewCrawl()
	if err != nil {
		log.Printf("db.PopNewCrawl() returned error %v", err)
		return nil
	}
	if importPath != "" {
		if pdoc, err := crawlDoc("new", importPath, nil, hasSubdirs, time.Time{}); pdoc == nil && err == nil {
			if err := db.AddBadCrawl(importPath); err != nil {
				log.Printf("ERROR db.AddBadCrawl(%q): %v", importPath, err)
			}
		}
		return nil
	}

	// Crawl existing doc.
	pdoc, pkgs, nextCrawl, err := db.Get("-")
	if err != nil {
		log.Printf("db.Get(\"-\") returned error %v", err)
		return nil
	}
	if pdoc == nil || nextCrawl.After(time.Now()) {
		return nil
	}
	if _, err = crawlDoc("crawl", pdoc.ImportPath, pdoc, len(pkgs) > 0, nextCrawl); err != nil {
		// Touch package so that crawl advances to next package.
		if err := db.SetNextCrawl(pdoc.ImportPath, time.Now().Add(viper.GetDuration(ConfigMaxAge)/3)); err != nil {
			log.Printf("ERROR db.SetNextCrawl(%q): %v", pdoc.ImportPath, err)
		}
	}
	return nil
}

func readGitHubUpdates() error {
	const key = "gitHubUpdates"
	var last string
	if err := db.GetGob(key, &last); err != nil {
		return err
	}
	last, names, err := gosrc.GetGitHubUpdates(httpClient, last)
	if err != nil {
		return err
	}

	for _, name := range names {
		log.Printf("bump crawl github.com/%s", name)
		if err := db.BumpCrawl("github.com/" + name); err != nil {
			log.Println("ERROR force crawl:", err)
		}
	}

	if err := db.PutGob(key, last); err != nil {
		return err
	}
	return nil
}
