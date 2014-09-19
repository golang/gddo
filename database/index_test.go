// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package database

import (
	"reflect"
	"sort"
	"testing"

	"github.com/golang/gddo/doc"
)

var indexTests = []struct {
	pdoc  *doc.Package
	terms []string
}{
	{&doc.Package{
		ImportPath:  "strconv",
		ProjectRoot: "",
		ProjectName: "Go",
		Name:        "strconv",
		Synopsis:    "Package strconv implements conversions to and from string representations of basic data types.",
		Doc:         "Package strconv implements conversions to and from string representations\nof basic data types.",
		Imports:     []string{"errors", "math", "unicode/utf8"},
		Funcs:       []*doc.Func{{}},
	},
		[]string{
			"bas",
			"convert",
			"dat",
			"import:errors",
			"import:math",
			"import:unicode/utf8",
			"project:go",
			"repres",
			"strconv",
			"string",
			"typ"},
	},
	{&doc.Package{
		ImportPath:  "github.com/user/repo/dir",
		ProjectRoot: "github.com/user/repo",
		ProjectName: "go-oauth",
		ProjectURL:  "https://github.com/user/repo/",
		Name:        "dir",
		Synopsis:    "Package dir implements a subset of the OAuth client interface as defined in RFC 5849.",
		Doc: "Package oauth implements a subset of the OAuth client interface as defined in RFC 5849.\n\n" +
			"This package assumes that the application writes request URL paths to the\nnetwork using " +
			"the encoding implemented by the net/url URL RequestURI method.\n" +
			"The HTTP client in the standard net/http package uses this encoding.",
		IsCmd: false,
		Imports: []string{
			"bytes",
			"crypto/hmac",
			"crypto/sha1",
			"encoding/base64",
			"encoding/binary",
			"errors",
			"fmt",
			"io",
			"io/ioutil",
			"net/http",
			"net/url",
			"regexp",
			"sort",
			"strconv",
			"strings",
			"sync",
			"time",
		},
		TestImports: []string{"bytes", "net/url", "testing"},
		Funcs:       []*doc.Func{{}},
	},
		[]string{
			"all:",
			"5849", "cly", "defin", "dir", "go",
			"import:bytes", "import:crypto/hmac", "import:crypto/sha1",
			"import:encoding/base64", "import:encoding/binary", "import:errors",
			"import:fmt", "import:io", "import:io/ioutil", "import:net/http",
			"import:net/url", "import:regexp", "import:sort", "import:strconv",
			"import:strings", "import:sync", "import:time", "interfac",
			"oau", "project:github.com/user/repo", "rfc", "subset",
		},
	},
}

func TestDocTerms(t *testing.T) {
	for _, tt := range indexTests {
		score := documentScore(tt.pdoc)
		terms := documentTerms(tt.pdoc, score)
		sort.Strings(terms)
		sort.Strings(tt.terms)
		if !reflect.DeepEqual(terms, tt.terms) {
			t.Errorf("documentTerms(%s)=%#v, want %#v", tt.pdoc.ImportPath, terms, tt.terms)
		}
	}
}

func TestExcludedPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"code.google.com/p/go.text/internal/ucd", true},
		{"code.google.com/p/go.text/internal", true},
		{"camlistore.org/third_party/bazil.org/fuse", true},
		{"bazil.org/fuse", false},
	}

	for _, tt := range tests {
		actual := isExcludedPath(tt.path)
		if actual != tt.expected {
			t.Errorf("isExcludedPath=%t, want %t for %s", actual, tt.expected, tt.path)
		}
	}
}
