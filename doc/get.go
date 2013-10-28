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
	"github.com/garyburd/gosrc"
	"go/doc"
	"net/http"
	"strings"
)

func Get(client *http.Client, importPath string, etag string) (*Package, error) {

	const versionPrefix = PackageVersion + "-"

	if strings.HasPrefix(etag, versionPrefix) {
		etag = etag[len(versionPrefix):]
	} else {
		etag = ""
	}

	dir, err := gosrc.Get(client, importPath, etag)
	if err != nil {
		return nil, err
	}

	pdoc, err := newPackage(dir)
	if err != nil {
		return pdoc, err
	}

	if pdoc.Synopsis == "" &&
		pdoc.Doc == "" &&
		!pdoc.IsCmd &&
		pdoc.Name != "" &&
		dir.ImportPath == dir.ProjectRoot &&
		len(pdoc.Errors) == 0 {
		project, err := gosrc.GetProject(client, dir.ResolvedPath)
		switch {
		case err == nil:
			pdoc.Synopsis = doc.Synopsis(project.Description)
		case gosrc.IsNotFound(err):
			// ok
		default:
			return nil, err
		}
	}

	return pdoc, nil
}
