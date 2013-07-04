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

package doc

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

// TODO: specify with command line flag
const repoRoot = "/tmp/gddo"

var urlTemplates = []struct {
	re       *regexp.Regexp
	template string
	lineFmt  string
}{
	{
		regexp.MustCompile(`^git\.gitorious\.org/(?P<repo>[^/]+/[^/]+)$`),
		"https://gitorious.org/{repo}/blobs/{tag}/{dir}{0}",
		"%s#line%d",
	},
	{
		regexp.MustCompile(`^camlistore\.org/r/p/(?P<repo>[^/]+)$`),
		"http://camlistore.org/code/?p={repo}.git;hb={tag};f={dir}{0}",
		"%s#l%d",
	},
}

// lookupURLTemplate finds an expand() template, match map and line number
// format for well known repositories.
func lookupURLTemplate(repo, dir, tag string) (string, map[string]string, string) {
	if strings.HasPrefix(dir, "/") {
		dir = dir[1:] + "/"
	}
	for _, t := range urlTemplates {
		if m := t.re.FindStringSubmatch(repo); m != nil {
			match := map[string]string{
				"dir": dir,
				"tag": tag,
			}
			for i, name := range t.re.SubexpNames() {
				if name != "" {
					match[name] = m[i]
				}
			}
			return t.template, match, t.lineFmt
		}
	}
	return "", nil, ""
}

type vcsCmd struct {
	schemes  []string
	download func([]string, string, string) (string, string, error)
}

var vcsCmds = map[string]*vcsCmd{
	"git": &vcsCmd{
		schemes:  []string{"http", "https", "git"},
		download: downloadGit,
	},
}

var lsremoteRe = regexp.MustCompile(`(?m)^([0-9a-f]{40})\s+refs/(?:tags|heads)/(.+)$`)

func downloadGit(schemes []string, repo, savedEtag string) (string, string, error) {
	var p []byte
	var scheme string
	for i := range schemes {
		cmd := exec.Command("git", "ls-remote", "--heads", "--tags", schemes[i]+"://"+repo+".git")
		log.Println(strings.Join(cmd.Args, " "))
		var err error
		p, err = cmd.Output()
		if err == nil {
			scheme = schemes[i]
			break
		}
	}

	if scheme == "" {
		return "", "", NotFoundError{"VCS not found"}
	}

	tags := make(map[string]string)
	for _, m := range lsremoteRe.FindAllSubmatch(p, -1) {
		tags[string(m[2])] = string(m[1])
	}

	tag, commit, err := bestTag(tags, "master")
	if err != nil {
		return "", "", err
	}

	etag := scheme + "-" + commit

	if etag == savedEtag {
		return "", "", ErrNotModified
	}

	dir := path.Join(repoRoot, repo+".git")
	p, err = ioutil.ReadFile(path.Join(dir, ".git/HEAD"))
	switch {
	case err != nil:
		if err := os.MkdirAll(dir, 0777); err != nil {
			return "", "", err
		}
		cmd := exec.Command("git", "clone", scheme+"://"+repo, dir)
		log.Println(strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	case string(bytes.TrimRight(p, "\n")) == commit:
		return tag, etag, nil
	default:
		cmd := exec.Command("git", "fetch")
		log.Println(strings.Join(cmd.Args, " "))
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return "", "", err
		}
	}

	cmd := exec.Command("git", "checkout", "--detach", "--force", commit)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "", "", err
	}

	return tag, etag, nil
}

var vcsPattern = regexp.MustCompile(`^(?P<repo>(?:[a-z0-9.\-]+\.)+[a-z0-9.\-]+(?::[0-9]+)?/[A-Za-z0-9_.\-/]*?)\.(?P<vcs>bzr|git|hg|svn)(?P<dir>/[A-Za-z0-9_.\-/]*)?$`)

func getVCSDoc(client *http.Client, match map[string]string, etagSaved string) (*Package, error) {
	cmd := vcsCmds[match["vcs"]]
	if cmd == nil {
		return nil, NotFoundError{expand("VCS not supported: {vcs}", match)}
	}

	scheme := match["scheme"]
	if scheme == "" {
		i := strings.Index(etagSaved, "-")
		if i > 0 {
			scheme = etagSaved[:i]
		}
	}

	schemes := cmd.schemes
	if scheme != "" {
		for i := range cmd.schemes {
			if cmd.schemes[i] == scheme {
				schemes = cmd.schemes[i : i+1]
				break
			}
		}
	}

	// Download and checkout.

	tag, etag, err := cmd.download(schemes, match["repo"], etagSaved)
	if err != nil {
		return nil, err
	}

	// Find source location.

	urlTemplate, urlMatch, lineFmt := lookupURLTemplate(match["repo"], match["dir"], tag)

	// Slurp source files.

	d := path.Join(repoRoot, expand("{repo}.{vcs}", match), match["dir"])
	f, err := os.Open(d)
	if err != nil {
		if os.IsNotExist(err) {
			err = NotFoundError{err.Error()}
		}
		return nil, err
	}
	fis, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	var files []*source
	for _, fi := range fis {
		if fi.IsDir() || !isDocFile(fi.Name()) {
			continue
		}
		b, err := ioutil.ReadFile(path.Join(d, fi.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, &source{
			name:      fi.Name(),
			browseURL: expand(urlTemplate, urlMatch, fi.Name()),
			data:      b,
		})
	}

	// Create the documentation.

	b := &builder{
		pdoc: &Package{
			LineFmt:     lineFmt,
			ImportPath:  match["importPath"],
			ProjectRoot: expand("{repo}.{vcs}", match),
			ProjectName: path.Base(match["repo"]),
			ProjectURL:  "",
			BrowseURL:   "",
			Etag:        etag,
			VCS:         match["vcs"],
		},
	}

	return b.build(files)
}
