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
	"net/url"
	"path"
	"regexp"
	"strings"
)

func importPathFromGoogleBrowse(m []string) string {
	project := m[1]
	dir := m[2]
	if dir == "" {
		dir = "/"
	} else if dir[len(dir)-1] == '/' {
		dir = dir[:len(dir)-1]
	}
	subrepo := ""
	if len(m[3]) > 0 {
		v, _ := url.ParseQuery(m[3][1:])
		subrepo = v.Get("repo")
		if len(subrepo) > 0 {
			subrepo = "." + subrepo
		}
	}
	if strings.HasPrefix(m[4], "#hg%2F") {
		d, _ := url.QueryUnescape(m[4][len("#hg%2f"):])
		if i := strings.IndexRune(d, '%'); i >= 0 {
			d = d[:i]
		}
		dir = dir + "/" + d
	}
	return "code.google.com/p/" + project + subrepo + dir
}

var browsePatterns = []struct {
	pat *regexp.Regexp
	fn  func([]string) string
}{
	{
		// GitHub tree  browser.
		regexp.MustCompile(`^https?://(github\.com/[^/]+/[^/]+)(?:/tree/[^/]+(/.*))?$`),
		func(m []string) string { return m[1] + m[2] },
	},
	{
		// GitHub file browser.
		regexp.MustCompile(`^https?://(github\.com/[^/]+/[^/]+)/blob/[^/]+/(.*)$`),
		func(m []string) string {
			d := path.Dir(m[2])
			if d == "." {
				return m[1]
			}
			return m[1] + "/" + d
		},
	},
	{
		// Bitbucket source borwser.
		regexp.MustCompile(`^https?://(bitbucket\.org/[^/]+/[^/]+)(?:/src/[^/]+(/[^?]+)?)?`),
		func(m []string) string { return m[1] + m[2] },
	},
	{
		// Google Project Hosting source browser.
		regexp.MustCompile(`^http:/+code\.google\.com/p/([^/]+)/source/browse(/[^?#]*)?(\?[^#]*)?(#.*)?$`),
		importPathFromGoogleBrowse,
	},
	{
		// Launchpad source browser.
		regexp.MustCompile(`^https?:/+bazaar\.(launchpad\.net/.*)/files$`),
		func(m []string) string { return m[1] },
	},
	{
		regexp.MustCompile(`^https?://(.+)$`),
		func(m []string) string { return strings.Trim(m[1], "/") },
	},
}

// isBrowserURL returns importPath and true if URL looks like a URL for a VCS
// source browser.
func isBrowseURL(s string) (importPath string, ok bool) {
	for _, c := range browsePatterns {
		if m := c.pat.FindStringSubmatch(s); m != nil {
			return c.fn(m), true
		}
	}
	return "", false
}
