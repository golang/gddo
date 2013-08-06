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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var gitHubProjectPat = regexp.MustCompile(`href="/([^/]+/[^/]+)/stargazers"`)
var gitHubUpdatedPat = regexp.MustCompile(`datetime="([^"]+)"`)

func readGitHubUpdates() (map[string]string, error) {
	updates := make(map[string]string)
	for i := 0; i < 2; i++ {
		resp, err := http.Get("https://github.com/languages/Go/updated?page=" + strconv.Itoa(i+1))
		if err != nil {
			return nil, err
		}
		p, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
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
				return nil, fmt.Errorf("updated not found for %s", ownerRepo)
			}
			updated := string(p[m[2]:m[3]])
			p = p[m[1]:]

			if _, found := updates[ownerRepo]; !found {
				updates[ownerRepo] = updated
			}
		}
	}
	if len(updates) == 0 {
		return nil, errors.New("no updates found")
	}
	return updates, nil
}

func crawlGitHubUpdates(interval time.Duration) {
	defer log.Println("ERROR, exiting github update scraper")

	const key = "ghupdates"
	sleep := false
	for {
		if sleep {
			time.Sleep(interval)
		}
		sleep = true

		updates, err := readGitHubUpdates()
		if err != nil {
			log.Println("ERROR github crawl:", err)
			continue
		}
		var prev map[string]string
		if err := db.GetGob(key, &prev); err != nil {
			log.Println("ERROR get prev updates:", err)
			continue
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
			log.Println("ERROR put updates:", err)
			continue
		}
	}
}
