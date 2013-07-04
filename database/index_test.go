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

package database

import (
	"github.com/garyburd/gddo/doc"
	"reflect"
	"sort"
	"testing"
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
