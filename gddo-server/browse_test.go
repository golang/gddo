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
	"testing"
)

var isBrowseURLTests = []struct {
	s          string
	importPath string
	ok         bool
}{
	{"https://github.com/garyburd/gddo/blob/master/doc/code.go", "github.com/garyburd/gddo/doc", true},
	{"https://github.com/garyburd/go-oauth/blob/master/.gitignore", "github.com/garyburd/go-oauth", true},
	{"https://bitbucket.org/user/repo/src/bd0b661a263e/p1/p2?at=default", "bitbucket.org/user/repo/p1/p2", true},
	{"https://bitbucket.org/user/repo/src", "bitbucket.org/user/repo", true},
	{"https://bitbucket.org/user/repo", "bitbucket.org/user/repo", true},
	{"https://github.com/user/repo", "github.com/user/repo", true},
	{"https://github.com/user/repo/tree/master/p1", "github.com/user/repo/p1", true},
	{"http://code.google.com/p/project", "code.google.com/p/project", true},
}

func TestIsBrowseURL(t *testing.T) {
	for _, tt := range isBrowseURLTests {
		importPath, ok := isBrowseURL(tt.s)
		if tt.ok {
			if importPath != tt.importPath || ok != true {
				t.Errorf("IsBrowseURL(%q) = %q, %v; want %q %v", tt.s, importPath, ok, tt.importPath, true)
			}
		} else if ok {
			t.Errorf("IsBrowseURL(%q) = %q, %v; want _, false", tt.s, importPath, ok)
		}
	}
}
