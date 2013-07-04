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

package doc

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"
)

// GetLocal gets the documentation from the localy installed
func GetLocal(importPath string, goroot, gopath string, browseURLFmt, lineFmt string) (*Package, error) {
	ctx := build.Default
	if goroot != "" {
		ctx.GOROOT = goroot
	}
	if gopath != "" {
		ctx.GOPATH = gopath
	}
	bpkg, err := ctx.Import(importPath, ".", build.FindOnly)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(bpkg.SrcRoot, filepath.FromSlash(importPath))
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var modTime time.Time
	var files []*source
	for _, fi := range fis {
		if fi.IsDir() || !isDocFile(fi.Name()) {
			continue
		}
		if fi.ModTime().After(modTime) {
			modTime = fi.ModTime()
		}
		b, err := ioutil.ReadFile(filepath.Join(dir, fi.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, &source{
			name:      fi.Name(),
			browseURL: fmt.Sprintf(browseURLFmt, fi.Name()),
			data:      b,
		})
	}
	b := &builder{
		pdoc: &Package{
			LineFmt:    lineFmt,
			ImportPath: importPath,
			Etag:       strconv.FormatInt(modTime.Unix(), 16),
		},
	}
	return b.build(files)
}
