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
	"path"
	"regexp"
	"strings"
)

var githubRawHeader = http.Header{"Accept": {"application/vnd.github-blob.raw"}}
var githubPattern = regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
var githubCred string

func SetGithubCredentials(id, secret string) {
	githubCred = "client_id=" + id + "&client_secret=" + secret
}

func getGithubDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	match["cred"] = githubCred

	var refs []*struct {
		Object struct {
			Type string
			Sha  string
			Url  string
		}
		Ref string
		Url string
	}

	err := httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/git/refs?{cred}", match), &refs)
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

	var tree struct {
		Tree []struct {
			Url  string
			Path string
			Type string
		}
		Url string
	}

	err = httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/git/trees/{tag}?recursive=1&{cred}", match), &tree)
	if err != nil {
		return nil, err
	}

	// Because Github API URLs are case-insensitive, we need to check that the
	// userRepo returned from Github matches the one that we are requesting.
	if !strings.HasPrefix(tree.Url, expand("https://api.github.com/repos/{owner}/{repo}/", match)) {
		return nil, NotFoundError{"Github import path has incorrect case."}
	}

	inTree := false
	dirPrefix := match["dir"]
	if dirPrefix != "" {
		dirPrefix = dirPrefix[1:] + "/"
	}
	var files []*source
	for _, node := range tree.Tree {
		if node.Type != "blob" || !strings.HasPrefix(node.Path, dirPrefix) {
			continue
		}
		inTree = true
		if d, f := path.Split(node.Path); d == dirPrefix && isDocFile(f) {
			files = append(files, &source{
				name:      f,
				browseURL: expand("https://github.com/{owner}/{repo}/blob/{tag}/{0}", match, node.Path),
				rawURL:    node.Url + "?" + githubCred,
			})
		}
	}

	if !inTree {
		return nil, NotFoundError{"Directory tree does not contain Go files."}
	}

	if err := fetchFiles(client, files, githubRawHeader); err != nil {
		return nil, err
	}

	browseURL := expand("https://github.com/{owner}/{repo}", match)
	if match["dir"] != "" {
		browseURL = expand("https://github.com/{owner}/{repo}/tree/{tag}{dir}", match)
	}

	b := &builder{
		pdoc: &Package{
			LineFmt:     "%s#L%d",
			ImportPath:  match["originalImportPath"],
			ProjectRoot: expand("github.com/{owner}/{repo}", match),
			ProjectName: match["repo"],
			ProjectURL:  expand("https://github.com/{owner}/{repo}", match),
			BrowseURL:   browseURL,
			Etag:        commit,
			VCS:         "git",
		},
	}

	return b.build(files)
}

func getGithubPresentation(client *http.Client, match map[string]string) (*Presentation, error) {

	match["cred"] = githubCred

	p, err := httpGetBytes(client, expand("https://api.github.com/repos/{owner}/{repo}/contents{dir}/{file}?{cred}", match), githubRawHeader)
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
			return fetchFiles(client, files, githubRawHeader)
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
