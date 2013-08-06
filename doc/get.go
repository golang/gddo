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

// Package doc fetches Go package documentation from version control services.
package doc

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
)

type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	return e.Message
}

func IsNotFound(err error) bool {
	_, ok := err.(NotFoundError)
	return ok
}

type RemoteError struct {
	Host string
	err  error
}

func (e *RemoteError) Error() string {
	return e.err.Error()
}

var (
	ErrNotModified = errors.New("package not modified")
	errNoMatch     = errors.New("no match")
)

// service represents a source code control service.
type service struct {
	pattern         *regexp.Regexp
	prefix          string
	get             func(*http.Client, map[string]string, string) (*Package, error)
	getPresentation func(*http.Client, map[string]string) (*Presentation, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{gitHubPattern, "github.com/", getGitHubDoc, getGitHubPresentation},
	{googlePattern, "code.google.com/", getGoogleDoc, getGooglePresentation},
	{bitbucketPattern, "bitbucket.org/", getBitbucketDoc, nil},
	{launchpadPattern, "launchpad.net/", getLaunchpadDoc, nil},
	{vcsPattern, "", getVCSDoc, nil},
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func fetchMeta(client *http.Client, importPath string) (map[string]string, error) {
	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	scheme := "https"
	resp, err := client.Get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = client.Get(scheme + "://" + uri)
		if err != nil {
			return nil, &RemoteError{strings.SplitN(importPath, "/", 2)[0], err}
		}
	}
	defer resp.Body.Close()
	return parseMeta(scheme, importPath, resp.Body)
}

func parseMeta(scheme, importPath string, r io.Reader) (map[string]string, error) {
	var match map[string]string

	d := xml.NewDecoder(r)
	d.Strict = false
metaScan:
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break metaScan
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				break metaScan
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				break metaScan
			}
			if !strings.EqualFold(t.Name.Local, "meta") ||
				attrValue(t.Attr, "name") != "go-import" {
				continue metaScan
			}
			f := strings.Fields(attrValue(t.Attr, "content"))
			if len(f) != 3 ||
				!strings.HasPrefix(importPath, f[0]) ||
				!(len(importPath) == len(f[0]) || importPath[len(f[0])] == '/') {
				continue metaScan
			}
			if match != nil {
				return nil, NotFoundError{"More than one <meta> found at " + scheme + "://" + importPath}
			}

			projectRoot, vcs, repo := f[0], f[1], f[2]

			repo = strings.TrimSuffix(repo, "."+vcs)
			i := strings.Index(repo, "://")
			if i < 0 {
				return nil, NotFoundError{"Bad repo URL in <meta>."}
			}
			proto := repo[:i]
			repo = repo[i+len("://"):]

			match = map[string]string{
				// Used in getVCSDoc, same as vcsPattern matches.
				"importPath": importPath,
				"repo":       repo,
				"vcs":        vcs,
				"dir":        importPath[len(projectRoot):],

				// Used in getVCSDoc
				"scheme": proto,

				// Used in getDynamic.
				"projectRoot": projectRoot,
				"projectName": path.Base(projectRoot),
				"projectURL":  scheme + "://" + projectRoot,
			}
		}
	}
	if match == nil {
		return nil, NotFoundError{"<meta> not found."}
	}
	return match, nil
}

// getDynamic gets a document from a service that is not statically known.
func getDynamic(client *http.Client, importPath, etag string) (*Package, error) {
	match, err := fetchMeta(client, importPath)
	if err != nil {
		return nil, err
	}

	if match["projectRoot"] != importPath {
		rootMatch, err := fetchMeta(client, match["projectRoot"])
		if err != nil {
			return nil, err
		}
		if rootMatch["projectRoot"] != match["projectRoot"] {
			return nil, NotFoundError{"Project root mismatch."}
		}
	}

	pdoc, err := getStatic(client, expand("{repo}{dir}", match), importPath, etag)
	if err == errNoMatch {
		pdoc, err = getVCSDoc(client, match, etag)
	}
	if err != nil {
		return nil, err
	}

	if pdoc != nil {
		pdoc.ProjectRoot = match["projectRoot"]
		pdoc.ProjectName = match["projectName"]
		pdoc.ProjectURL = match["projectURL"]
	}

	return pdoc, err
}

// getStatic gets a document from a statically known service. getStatic
// returns errNoMatch if the import path is not recognized.
func getStatic(client *http.Client, importPath, originalImportPath, etag string) (*Package, error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(importPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(importPath)
		if m == nil {
			if s.prefix != "" {
				return nil, NotFoundError{"Import path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": importPath, "originalImportPath": originalImportPath}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(client, match, etag)
	}
	return nil, errNoMatch
}

func Get(client *http.Client, importPath string, etag string) (pdoc *Package, err error) {

	const versionPrefix = PackageVersion + "-"

	if strings.HasPrefix(etag, versionPrefix) {
		etag = etag[len(versionPrefix):]
	} else {
		etag = ""
	}

	switch {
	case IsGoRepoPath(importPath):
		pdoc, err = getStandardDoc(client, importPath, etag)
	case IsValidRemotePath(importPath):
		pdoc, err = getStatic(client, importPath, importPath, etag)
		if err == errNoMatch {
			pdoc, err = getDynamic(client, importPath, etag)
		}
	default:
		err = errNoMatch
	}

	if err == errNoMatch {
		err = NotFoundError{"Import path not valid:"}
	}

	if pdoc != nil {
		pdoc.Etag = versionPrefix + pdoc.Etag
		if pdoc.ImportPath != importPath {
			return nil, fmt.Errorf("Get: pdoc.ImportPath = %q, want %q", pdoc.ImportPath, importPath)
		}
	}

	return pdoc, err
}

// GetPresentation gets a presentation from the the given path.
func GetPresentation(client *http.Client, importPath string) (*Presentation, error) {
	ext := path.Ext(importPath)
	if ext != ".slide" && ext != ".article" {
		return nil, NotFoundError{"unknown file extension."}
	}

	importPath, file := path.Split(importPath)
	importPath = strings.TrimSuffix(importPath, "/")
	for _, s := range services {
		if s.getPresentation == nil || !strings.HasPrefix(importPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(importPath)
		if m == nil {
			if s.prefix != "" {
				return nil, NotFoundError{"path prefix matches known service, but regexp does not."}
			}
			continue
		}
		match := map[string]string{"importPath": importPath, "file": file}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.getPresentation(client, match)
	}
	return nil, errNoMatch
}
