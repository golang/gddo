// Copyright 2011 Gary Burd
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

package doc

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	gitHubRawHeader     = http.Header{"Accept": {"application/vnd.github-blob.raw"}}
	gitHubPreviewHeader = http.Header{"Accept": {"application/vnd.github.preview"}}
	gitHubPattern       = regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
	gitHubCred          string
)

func SetGitHubCredentials(id, secret string) {
	gitHubCred = "client_id=" + id + "&client_secret=" + secret
}

func getGitHubDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	match["cred"] = gitHubCred

	var refs []*struct {
		Object struct {
			Type string
			Sha  string
			URL  string
		}
		Ref string
		URL string
	}

	err := httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/git/refs?{cred}", match), nil, &refs)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for _, ref := range refs {
		switch {
		case strings.HasPrefix(ref.Ref, "refs/heads/"):
			tags[ref.Ref[len("refs/heads/"):]] = ref.Object.Sha
		case strings.HasPrefix(ref.Ref, "refs/tags/"):
			tags[ref.Ref[len("refs/tags/"):]] = ref.Object.Sha
		}
	}

	var commit string
	match["tag"], commit, err = bestTag(tags, "master")
	if err != nil {
		return nil, err
	}

	if commit == savedEtag {
		return nil, ErrNotModified
	}

	var contents []*struct {
		Type    string
		Name    string
		GitURL  string `json:"git_url"`
		HTMLURL string `json:"html_url"`
	}

	err = httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/contents{dir}?ref={tag}&{cred}", match), nil, &contents)
	if err != nil {
		return nil, err
	}

	if len(contents) == 0 {
		return nil, NotFoundError{"No files in directory."}
	}

	// Because Github API URLs are case-insensitive, we check that the owner
	// and repo returned from Github matches the one that we are requesting.
	if !strings.HasPrefix(contents[0].GitURL, expand("https://api.github.com/repos/{owner}/{repo}/", match)) {
		return nil, NotFoundError{"Github import path has incorrect case."}
	}

	var files []*source
	var subdirs []string

	for _, item := range contents {
		switch {
		case item.Type == "dir":
			if isValidPathElement(item.Name) {
				subdirs = append(subdirs, item.Name)
			}
		case isDocFile(item.Name):
			files = append(files, &source{
				name:      item.Name,
				browseURL: item.HTMLURL,
				rawURL:    item.GitURL + "?" + gitHubCred,
			})
		}
	}

	if err := fetchFiles(client, files, gitHubRawHeader); err != nil {
		return nil, err
	}

	browseURL := expand("https://github.com/{owner}/{repo}", match)
	if match["dir"] != "" {
		browseURL = expand("https://github.com/{owner}/{repo}/tree/{tag}{dir}", match)
	}

	b := &builder{
		pdoc: &Package{
			LineFmt:        "%s#L%d",
			ImportPath:     match["originalImportPath"],
			ProjectRoot:    expand("github.com/{owner}/{repo}", match),
			ProjectName:    match["repo"],
			ProjectURL:     expand("https://github.com/{owner}/{repo}", match),
			BrowseURL:      browseURL,
			Etag:           commit,
			VCS:            "git",
			Subdirectories: subdirs,
		},
	}

	return b.build(files)
}

func getGitHubPresentation(client *http.Client, match map[string]string) (*Presentation, error) {

	match["cred"] = gitHubCred

	p, err := httpGetBytes(client, expand("https://api.github.com/repos/{owner}/{repo}/contents{dir}/{file}?{cred}", match), gitHubRawHeader)
	if err != nil {
		return nil, err
	}

	apiBase, err := url.Parse(expand("https://api.github.com/repos/{owner}/{repo}/contents{dir}/?{cred}", match))
	if err != nil {
		return nil, err
	}
	rawBase, err := url.Parse(expand("https://raw.github.com/{owner}/{repo}/master{dir}/", match))
	if err != nil {
		return nil, err
	}

	b := &presBuilder{
		data:     p,
		filename: match["file"],
		fetch: func(files []*source) error {
			for _, f := range files {
				u, err := apiBase.Parse(f.name)
				if err != nil {
					return err
				}
				u.RawQuery = apiBase.RawQuery
				f.rawURL = u.String()
			}
			return fetchFiles(client, files, gitHubRawHeader)
		},
		resolveURL: func(fname string) string {
			u, err := rawBase.Parse(fname)
			if err != nil {
				return "/notfound"
			}
			if strings.HasSuffix(fname, ".svg") {
				u.Host = "rawgithub.com"
			}
			return u.String()
		},
	}

	return b.build()
}

type GitHubUpdate struct {
	OwnerRepo string
	Fork      bool
}

func GetGitHubUpdates(client *http.Client, last string) (string, []GitHubUpdate, error) {
	if last == "" {
		last = time.Now().Add(-24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	}
	u := "https://api.github.com/search/repositories?order=asc&sort=updated&q=language:Go+pushed:>" + last
	if gitHubCred != "" {
		u += "&" + gitHubCred
	}
	var updates struct {
		Items []struct {
			FullName string `json:"full_name"`
			Fork     bool
			PushedAt string `json:"pushed_at"`
		}
	}
	err := httpGetJSON(client, u, gitHubPreviewHeader, &updates)
	if err != nil {
		return "", nil, err
	}
	var result []GitHubUpdate
	for _, item := range updates.Items {
		result = append(result, GitHubUpdate{OwnerRepo: item.FullName, Fork: item.Fork})
		if item.PushedAt > last {
			last = item.PushedAt
		}
	}
	return last, result, nil
}
