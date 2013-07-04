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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
)

type byHash []byte

func (p byHash) Len() int { return len(p) / md5.Size }
func (p byHash) Less(i, j int) bool {
	return -1 == bytes.Compare(p[i*md5.Size:(i+1)*md5.Size], p[j*md5.Size:(j+1)*md5.Size])
}
func (p byHash) Swap(i, j int) {
	var temp [md5.Size]byte
	copy(temp[:], p[i*md5.Size:])
	copy(p[i*md5.Size:(i+1)*md5.Size], p[j*md5.Size:])
	copy(p[j*md5.Size:], temp[:])
}

var launchpadPattern = regexp.MustCompile(`^launchpad\.net/(?P<repo>(?P<project>[a-z0-9A-Z_.\-]+)(?P<series>/[a-z0-9A-Z_.\-]+)?|~[a-z0-9A-Z_.\-]+/(\+junk|[a-z0-9A-Z_.\-]+)/[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]+)*$`)

func getLaunchpadDoc(client *http.Client, match map[string]string, savedEtag string) (*Package, error) {

	if match["project"] != "" && match["series"] != "" {
		rc, err := httpGet(client, expand("https://code.launchpad.net/{project}{series}/.bzr/branch-format", match), nil)
		switch {
		case err == nil:
			rc.Close()
			// The structure of the import path is launchpad.net/{root}/{dir}.
		case IsNotFound(err):
			// The structure of the import path is is launchpad.net/{project}/{dir}.
			match["repo"] = match["project"]
			match["dir"] = expand("{series}{dir}", match)
		default:
			return nil, err
		}
	}

	p, err := httpGetBytes(client, expand("https://bazaar.launchpad.net/+branch/{repo}/tarball", match), nil)
	if err != nil {
		return nil, err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(p))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var hash []byte
	inTree := false
	dirPrefix := expand("+branch/{repo}{dir}/", match)
	var files []*source
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		d, f := path.Split(h.Name)
		if !isDocFile(f) {
			continue
		}
		b := make([]byte, h.Size)
		if _, err := io.ReadFull(tr, b); err != nil {
			return nil, err
		}

		m := md5.New()
		m.Write(b)
		hash = m.Sum(hash)

		if !strings.HasPrefix(h.Name, dirPrefix) {
			continue
		}
		inTree = true
		if d == dirPrefix {
			files = append(files, &source{
				name:      f,
				browseURL: expand("http://bazaar.launchpad.net/+branch/{repo}/view/head:{dir}/{0}", match, f),
				data:      b})
		}
	}

	if !inTree {
		return nil, NotFoundError{"Directory tree does not contain Go files."}
	}

	sort.Sort(byHash(hash))
	m := md5.New()
	m.Write(hash)
	hash = m.Sum(hash[:0])
	etag := hex.EncodeToString(hash)
	if etag == savedEtag {
		return nil, ErrNotModified
	}

	b := &builder{
		pdoc: &Package{
			LineFmt:     "%s#L%d",
			ImportPath:  match["originalImportPath"],
			ProjectRoot: expand("launchpad.net/{repo}", match),
			ProjectName: match["repo"],
			ProjectURL:  expand("https://launchpad.net/{repo}/", match),
			BrowseURL:   expand("http://bazaar.launchpad.net/+branch/{repo}/view/head:{dir}/", match),
			Etag:        etag,
			VCS:         "bzr",
		},
	}
	return b.build(files)
}
