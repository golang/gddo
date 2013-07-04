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
	"testing"
)

var goodImportPaths = []string{
	"github.com/user/repo",
	"github.com/user/repo/src/pkg/compress/somethingelse",
	"github.com/user/repo/src/compress/gzip",
	"github.com/user/repo/src/pkg",
	"camlistore.org/r/p/camlistore",
	"example.com/foo.git",
	"launchpad.net/~user/foo/trunk",
	"launchpad.net/~user/+junk/version",
}

var badImportPaths = []string{
	"foobar",
	"foo.",
	".bar",
	"favicon.ico",
	"exmpple.com",
	"github.com/user/repo/testdata/x",
	"github.com/user/repo/_ignore/x",
	"github.com/user/repo/.ignore/x",
}

func TestIsValidRemotePath(t *testing.T) {
	for _, importPath := range goodImportPaths {
		if !IsValidRemotePath(importPath) {
			t.Errorf("isBadImportPath(%q) -> true, want false", importPath)
		}
	}
	for _, importPath := range badImportPaths {
		if IsValidRemotePath(importPath) {
			t.Errorf("isBadImportPath(%q) -> false, want true", importPath)
		}
	}
}
