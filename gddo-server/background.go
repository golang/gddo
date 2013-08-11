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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var backgroundTasks = []*struct {
	name     string
	fn       func() error
	interval *time.Duration
	next     time.Time
}{
	{
		name:     "GitHub updates",
		fn:       readGitHubUpdates,
		interval: flag.Duration("github_interval", 0, "Github updates crawler sleeps for this duration between fetches. Zero disables the crawler."),
	},
	{
		name:     "Crawl",
		fn:       doCrawl,
		interval: flag.Duration("crawl_interval", 0, "Package updater sleeps for this duration between package updates. Zero disables updates."),
	},
}

func runBackgroundTasks() {
	defer log.Println("ERROR: Background exiting!")

	sleep := time.Minute
	for _, task := range backgroundTasks {
		if *task.interval > 0 && sleep > *task.interval {
			sleep = *task.interval
		}
	}

	for {
		for _, task := range backgroundTasks {
			start := time.Now()
			if *task.interval > 0 && start.After(task.next) {
				if err := task.fn(); err != nil {
					log.Printf("Task %s: %v", task.name, err)
				}
				task.next = time.Now().Add(*task.interval)
			}
		}
		time.Sleep(sleep)
	}
}

func doCrawl() error {
	// Look for new package to crawl.
	importPath, err := db.GetNewCrawl()
	if err != nil {
		log.Printf("db.GetNewCrawl() returned error %v", err)
		return nil
	}
	if importPath != "" {
		if pdoc, err := crawlDoc("new", importPath, nil, false, time.Time{}); err != nil || pdoc == nil {
			if err := db.SetBadCrawl(importPath); err != nil {
				log.Printf("ERROR db.SetBadCrawl(%q): %v", importPath, err)
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
		if err := db.SetNextCrawlEtag(pdoc.ProjectRoot, pdoc.Etag, time.Now().Add(*maxAge/3)); err != nil {
			log.Printf("ERROR db.TouchLastCrawl(%q): %v", pdoc.ImportPath, err)
		}
	}
	return nil
}

var gitHubProjectPat = regexp.MustCompile(`href="/([^/]+/[^/]+)/stargazers"`)
var gitHubUpdatedPat = regexp.MustCompile(`datetime="([^"]+)"`)

func readGitHubUpdates() error {
	updates := make(map[string]string)
	for i := 0; i < 2; i++ {
		resp, err := http.Get("https://github.com/languages/Go/updated?page=" + strconv.Itoa(i+1))
		if err != nil {
			return err
		}
		p, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		for {
			m := gitHubProjectPat.FindSubmatchIndex(p)
			if m == nil {
				break
			}
			ownerRepo := string(p[m[2]:m[3]])
			p = p[m[1]:]

			m = gitHubUpdatedPat.FindSubmatchIndex(p)
			if m == nil {
				return fmt.Errorf("updated not found for %s", ownerRepo)
			}
			updated := string(p[m[2]:m[3]])
			p = p[m[1]:]

			if _, found := updates[ownerRepo]; !found {
				updates[ownerRepo] = updated
			}
		}
	}
	if len(updates) == 0 {
		return errors.New("no updates found")
	}

	const key = "ghupdates"
	var prev map[string]string
	if err := db.GetGob(key, &prev); err != nil {
		return err
	}
	if prev == nil {
		prev = make(map[string]string)
	}
	for ownerRepo, t := range updates {
		if prev[ownerRepo] != t {
			d := time.Duration(0)
			if prev[ownerRepo] != "" {
				// Delay crawl if repo was updated recently.
				d = time.Hour
			}
			log.Printf("Set next crawl for %s to %v from now", ownerRepo, d)
			if err := db.SetNextCrawl("github.com/"+ownerRepo, time.Now().Add(d)); err != nil {
				log.Println("ERROR set next crawl:", err)
			}
		}
	}
	if err := db.PutGob(key, updates); err != nil {
		return err
	}
	return nil
}
