// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

// Package gosrc fetches Go package source code from version control services.
package gosrc

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// File represents a file.
type File struct {
	// File name with no directory.
	Name string

	// Contents of the file.
	Data []byte

	// Location of file on version control service website.
	BrowseURL string
}

// Directory describes a directory on a version control service.
type Directory struct {
	// The import path for this package.
	ImportPath string

	// Import path of package after resolving go-import meta tags, if any.
	ResolvedPath string

	// Import path prefix for all packages in the project.
	ProjectRoot string

	// Name of the project.
	ProjectName string

	// Project home page.
	ProjectURL string

	// Version control system: git, hg, bzr, ...
	VCS string

	// Version control: belongs to a dead end fork
	DeadEndFork bool

	// Cache validation tag. This tag is not necessarily an HTTP entity tag.
	// The tag is "" if there is no meaningful cache validation for the VCS.
	Etag string

	// Files.
	Files []*File

	// Subdirectories, not guaranteed to contain Go code.
	Subdirectories []string

	// Location of directory on version control service website.
	BrowseURL string

	// Format specifier for link to source line. Example: "%s#L%d"
	LineFmt string
}

// Project represents a repository.
type Project struct {
	Description string
}

// NotFoundError indicates that the directory or presentation was not found.
type NotFoundError struct {
	// Diagnostic message describing why the directory was not found.
	Message string

	// Redirect specifies the path where package can be found.
	Redirect string
}

func (e NotFoundError) Error() string {
	return e.Message
}

// IsNotFound returns true if err is of type NotFoundError.
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

// ErrNotModified indicates that the directory matches the specified etag.
var ErrNotModified = errors.New("package not modified")

var errNoMatch = errors.New("no match")

// service represents a source code control service.
type service struct {
	pattern         *regexp.Regexp
	prefix          string
	get             func(*http.Client, map[string]string, string) (*Directory, error)
	getPresentation func(*http.Client, map[string]string) (*Presentation, error)
	getProject      func(*http.Client, map[string]string) (*Project, error)
}

var services []*service

func addService(s *service) {
	if s.prefix == "" {
		services = append(services, s)
	} else {
		services = append([]*service{s}, services...)
	}
}

func (s *service) match(importPath string) (map[string]string, error) {
	if !strings.HasPrefix(importPath, s.prefix) {
		return nil, nil
	}
	m := s.pattern.FindStringSubmatch(importPath)
	if m == nil {
		if s.prefix != "" {
			return nil, NotFoundError{Message: "Import path prefix matches known service, but regexp does not."}
		}
		return nil, nil
	}
	match := map[string]string{"importPath": importPath}
	for i, n := range s.pattern.SubexpNames() {
		if n != "" {
			match[n] = m[i]
		}
	}
	return match, nil
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

	c := httpClient{client: client}
	scheme := "https"
	resp, err := c.get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = c.get(scheme + "://" + uri)
		if err != nil {
			return nil, err
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
				return nil, NotFoundError{Message: "More than one <meta> found at " + scheme + "://" + importPath}
			}

			projectRoot, vcs, repo := f[0], f[1], f[2]

			repo = strings.TrimSuffix(repo, "."+vcs)
			i := strings.Index(repo, "://")
			if i < 0 {
				return nil, NotFoundError{Message: "Bad repo URL in <meta>."}
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
		return nil, NotFoundError{Message: "<meta> not found."}
	}
	return match, nil
}

var getVCSDirFn = func(client *http.Client, m map[string]string, etag string) (*Directory, error) {
	return nil, errNoMatch
}

// getDynamic gets a directory from a service that is not statically known.
func getDynamic(client *http.Client, importPath, etag string) (*Directory, error) {
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
			return nil, NotFoundError{Message: "Project root mismatch."}
		}
	}

	dir, err := getStatic(client, expand("{repo}{dir}", match), etag)
	if err == errNoMatch {
		dir, err = getVCSDirFn(client, match, etag)
	}
	if err != nil {
		return nil, err
	}

	if dir != nil {
		dir.ImportPath = importPath
		dir.ProjectRoot = match["projectRoot"]
		dir.ProjectName = match["projectName"]
		dir.ProjectURL = match["projectURL"]
		if dir.ResolvedPath == "" {
			dir.ResolvedPath = dir.ImportPath
		}
	}

	return dir, err
}

// getStatic gets a diretory from a statically known service. getStatic
// returns errNoMatch if the import path is not recognized.
func getStatic(client *http.Client, importPath, etag string) (*Directory, error) {
	for _, s := range services {
		if s.get == nil {
			continue
		}
		match, err := s.match(importPath)
		if err != nil {
			return nil, err
		}
		if match != nil {
			dir, err := s.get(client, match, etag)
			if dir != nil {
				dir.ImportPath = importPath
				dir.ResolvedPath = importPath
			}
			return dir, err
		}
	}
	return nil, errNoMatch
}

func Get(client *http.Client, importPath string, etag string) (dir *Directory, err error) {
	switch {
	case localPath != "":
		dir, err = getLocal(importPath)
	case IsGoRepoPath(importPath):
		dir, err = getStandardDir(client, importPath, etag)
	case IsValidRemotePath(importPath):
		dir, err = getStatic(client, importPath, etag)
		if err == errNoMatch {
			dir, err = getDynamic(client, importPath, etag)
		}
	default:
		err = errNoMatch
	}

	if err == errNoMatch {
		err = NotFoundError{Message: "Import path not valid:"}
	}

	return dir, err
}

// GetPresentation gets a presentation from the the given path.
func GetPresentation(client *http.Client, importPath string) (*Presentation, error) {
	ext := path.Ext(importPath)
	if ext != ".slide" && ext != ".article" {
		return nil, NotFoundError{Message: "unknown file extension."}
	}

	importPath, file := path.Split(importPath)
	importPath = strings.TrimSuffix(importPath, "/")
	for _, s := range services {
		if s.getPresentation == nil {
			continue
		}
		match, err := s.match(importPath)
		if err != nil {
			return nil, err
		}
		if match != nil {
			match["file"] = file
			return s.getPresentation(client, match)
		}
	}
	return nil, NotFoundError{Message: "path does not match registered service"}
}

// GetProject gets information about a repository.
func GetProject(client *http.Client, importPath string) (*Project, error) {
	for _, s := range services {
		if s.getProject == nil {
			continue
		}
		match, err := s.match(importPath)
		if err != nil {
			return nil, err
		}
		if match != nil {
			return s.getProject(client, match)
		}
	}
	return nil, NotFoundError{Message: "path does not match registered service"}
}
