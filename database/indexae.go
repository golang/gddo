// Copyright 2016 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package database

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/context"
	"google.golang.org/appengine/search"

	"github.com/golang/gddo/doc"
)

// PackageDocument defines the data structure used to represent a package document
// in the search index.
type PackageDocument struct {
	Name        search.Atom
	Path        string
	Synopsis    string
	Score       float64
	ImportCount float64
	Stars       float64
	Fork        search.Atom
}

// PutIndex creates or updates a package entry in the search index. id identifies the document in the index.
// If pdoc is non-nil, PutIndex will update the package's name, path and synopsis supplied by pdoc.
// pdoc must be non-nil for a package's first call to PutIndex.
// PutIndex updates the Score to score, if non-negative.
func PutIndex(c context.Context, pdoc *doc.Package, id string, score float64, importCount int) error {
	if id == "" {
		return errors.New("indexae: no id assigned")
	}
	idx, err := search.Open("packages")
	if err != nil {
		return err
	}

	var pkg PackageDocument
	if err := idx.Get(c, id, &pkg); err != nil {
		if err != search.ErrNoSuchDocument {
			return err
		} else if pdoc == nil {
			// Cannot update a non-existing document.
			return errors.New("indexae: cannot create new document with nil pdoc")
		}
		// No such document in the index, fall through.
	}

	// Update document information accordingly.
	if pdoc != nil {
		pkg.Name = search.Atom(pdoc.Name)
		pkg.Path = pdoc.ImportPath
		pkg.Synopsis = pdoc.Synopsis
		pkg.Stars = float64(pdoc.Stars)
		var fork string
		if forkAvailable(pdoc.ImportPath) {
			fork = fmt.Sprint(pdoc.Fork) // "true" or "false"
		}
		pkg.Fork = search.Atom(fork)
	}
	if score >= 0 {
		pkg.Score = score
	}
	pkg.ImportCount = float64(importCount)

	if _, err := idx.Put(c, id, &pkg); err != nil {
		return err
	}
	return nil
}

func forkAvailable(p string) bool {
	return strings.HasPrefix(p, "github.com") || strings.HasPrefix(p, "bitbucket.org")
}

// Search searches the packages index for a given query. A path-like query string
// will be passed in unchanged, whereas single words will be stemmed.
func Search(c context.Context, q string) ([]Package, error) {
	index, err := search.Open("packages")
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	opt := &search.SearchOptions{
		Sort: &search.SortOptions{
			Expressions: []search.SortExpression{
				{Expr: "Score * log(10 + ImportCount)"},
			},
		},
	}
	for it := index.Search(c, parseQuery2(q), opt); ; {
		var pd PackageDocument
		_, err := it.Next(&pd)
		if err == search.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		pkg := Package{
			Path:        pd.Path,
			ImportCount: int(pd.ImportCount),
			Synopsis:    pd.Synopsis,
		}
		if pd.Fork == "true" {
			pkg.Fork = true
		}
		if pd.Stars > 0 {
			pkg.Stars = int(pd.Stars)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

func parseQuery2(q string) string {
	var buf bytes.Buffer
	for _, s := range strings.FieldsFunc(q, isTermSep2) {
		if strings.ContainsAny(s, "./") {
			// Quote terms with / or . for path like query.
			fmt.Fprintf(&buf, "%q ", s)
		} else {
			// Stem for single word terms.
			fmt.Fprintf(&buf, "~%v ", s)
		}
	}
	return buf.String()
}

func isTermSep2(r rune) bool {
	return unicode.IsSpace(r) ||
		r != '.' && r != '/' && unicode.IsPunct(r) ||
		unicode.IsSymbol(r)
}
