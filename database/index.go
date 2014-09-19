// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package database

import (
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/golang/gddo/doc"
	"github.com/golang/gddo/gosrc"
)

func isStandardPackage(path string) bool {
	return strings.Index(path, ".") < 0
}

func isTermSep(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func normalizeProjectRoot(projectRoot string) string {
	if projectRoot == "" {
		return "go"
	}
	return projectRoot
}

var synonyms = map[string]string{
	"redis":    "redisdb", // append db to avoid stemming to 'red'
	"rand":     "random",
	"postgres": "postgresql",
	"mongo":    "mongodb",
}

func term(s string) string {
	s = strings.ToLower(s)
	if x, ok := synonyms[s]; ok {
		s = x
	}
	return stem(s)
}

var httpPat = regexp.MustCompile(`https?://\S+`)

func documentTerms(pdoc *doc.Package, score float64) []string {

	terms := make(map[string]bool)

	// Project root

	projectRoot := normalizeProjectRoot(pdoc.ProjectRoot)
	terms["project:"+projectRoot] = true

	if strings.HasPrefix(pdoc.ImportPath, "code.google.com/p/go.") {
		terms["project:subrepo"] = true
	}

	// Imports

	for _, path := range pdoc.Imports {
		if gosrc.IsValidPath(path) {
			terms["import:"+path] = true
		}
	}

	if score > 0 {

		if isStandardPackage(pdoc.ImportPath) {
			for _, term := range parseQuery(pdoc.ImportPath) {
				terms[term] = true
			}
		} else {
			terms["all:"] = true
			for _, term := range parseQuery(pdoc.ProjectName) {
				terms[term] = true
			}
			for _, term := range parseQuery(pdoc.Name) {
				terms[term] = true
			}
		}

		// Synopsis

		synopsis := httpPat.ReplaceAllLiteralString(pdoc.Synopsis, "")
		for i, s := range strings.FieldsFunc(synopsis, isTermSep) {
			s = strings.ToLower(s)
			if !stopWord[s] && (i > 3 || s != "package") {
				terms[term(s)] = true
			}
		}
	}

	result := make([]string, 0, len(terms))
	for term := range terms {
		result = append(result, term)
	}
	return result
}

func isExcludedPath(path string) bool {
	if strings.HasSuffix(path, ".go") ||
		strings.HasPrefix(path, "gist.github.com/") ||
		strings.Contains(path, "/internal/") ||
		strings.HasSuffix(path, "/internal") ||
		strings.Contains(path, "/third_party/") {
		return true
	}
	return false
}

func documentScore(pdoc *doc.Package) float64 {
	if pdoc.Name == "" ||
		pdoc.IsCmd ||
		len(pdoc.Errors) > 0 ||
		isExcludedPath(pdoc.ImportPath) {
		return 0
	}

	for _, p := range pdoc.Imports {
		if strings.HasSuffix(p, ".go") {
			return 0
		}
	}

	if !pdoc.Truncated &&
		len(pdoc.Consts) == 0 &&
		len(pdoc.Vars) == 0 &&
		len(pdoc.Funcs) == 0 &&
		len(pdoc.Types) == 0 &&
		len(pdoc.Examples) == 0 {
		return 0
	}

	r := 1.0
	if pdoc.Doc == "" || pdoc.Synopsis == "" {
		r *= 0.95
	}
	if path.Base(pdoc.ImportPath) != pdoc.Name {
		r *= 0.9
	}
	for i := 0; i < strings.Count(pdoc.ImportPath[len(pdoc.ProjectRoot):], "/"); i++ {
		r *= 0.99
	}
	if strings.Index(pdoc.ImportPath[len(pdoc.ProjectRoot):], "/src/") > 0 {
		r *= 0.95
	}
	return r
}

func parseQuery(q string) []string {
	var terms []string
	q = strings.ToLower(q)
	for _, s := range strings.FieldsFunc(q, isTermSep) {
		if !stopWord[s] {
			terms = append(terms, term(s))
		}
	}
	return terms
}
