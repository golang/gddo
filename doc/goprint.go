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

// +build ignore

// Command astprint prints the AST for a file.
//
// Usage: go run asprint.go fname
package main

import (
	"flag"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

func importer(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg == nil {
		name := path[strings.LastIndex(path, "/")+1:]
		pkg = ast.NewObj(ast.Pkg, name)
		pkg.Data = ast.NewScope(nil) // required by ast.NewPackage for dot-import
		imports[path] = pkg
	}
	return pkg, nil
}

func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatal("Usage: go run goprint.go path")
	}
	bpkg, err := build.Default.Import(flag.Args()[0], ".", 0)
	if err != nil {
		log.Fatal(err)
	}
	fset := token.NewFileSet()
	files := make(map[string]*ast.File)
	for _, fname := range bpkg.GoFiles {
		p, err := ioutil.ReadFile(filepath.Join(bpkg.SrcRoot, bpkg.ImportPath, fname))
		if err != nil {
			log.Fatal(err)
		}
		file, err := parser.ParseFile(fset, fname, p, parser.ParseComments)
		if err != nil {
			log.Fatal(err)
		}
		files[fname] = file
	}
	c := spew.NewDefaultConfig()
	c.DisableMethods = true
	apkg, _ := ast.NewPackage(fset, files, importer, nil)
	c.Dump(apkg)
	ast.Print(fset, apkg)
	dpkg := doc.New(apkg, bpkg.ImportPath, 0)
	c.Dump(dpkg)
}
