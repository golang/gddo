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
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/garyburd/gosrc"
)

// This list of deprecated exports is used to find code that has not been
// updated for Go 1.
var deprecatedExports = map[string][]string{
	`"bytes"`:         {"Add"},
	`"crypto/aes"`:    {"Cipher"},
	`"crypto/hmac"`:   {"NewSHA1", "NewSHA256"},
	`"crypto/rand"`:   {"Seed"},
	`"encoding/json"`: {"MarshalForHTML"},
	`"encoding/xml"`:  {"Marshaler", "NewParser", "Parser"},
	`"html"`:          {"NewTokenizer", "Parse"},
	`"image"`:         {"Color", "NRGBAColor", "RGBAColor"},
	`"io"`:            {"Copyn"},
	`"log"`:           {"Exitf"},
	`"math"`:          {"Fabs", "Fmax", "Fmod"},
	`"os"`:            {"Envs", "Error", "Getenverror", "NewError", "Time", "UnixSignal", "Wait"},
	`"reflect"`:       {"MapValue", "Typeof"},
	`"runtime"`:       {"UpdateMemStats"},
	`"strconv"`:       {"Atob", "Atof32", "Atof64", "AtofN", "Atoi64", "Atoui", "Atoui64", "Btoui64", "Ftoa64", "Itoa64", "Uitoa", "Uitoa64"},
	`"time"`:          {"LocalTime", "Nanoseconds", "NanosecondsToLocalTime", "Seconds", "SecondsToLocalTime", "SecondsToUTC"},
	`"unicode/utf8"`:  {"NewString"},
}

type vetVisitor struct {
	errors map[string]token.Pos
}

func (v *vetVisitor) Visit(n ast.Node) ast.Visitor {
	if sel, ok := n.(*ast.SelectorExpr); ok {
		if x, _ := sel.X.(*ast.Ident); x != nil {
			if obj := x.Obj; obj != nil && obj.Kind == ast.Pkg {
				if spec, _ := obj.Decl.(*ast.ImportSpec); spec != nil {
					for _, name := range deprecatedExports[spec.Path.Value] {
						if name == sel.Sel.Name {
							v.errors[fmt.Sprintf("%s.%s not found", spec.Path.Value, sel.Sel.Name)] = n.Pos()
							return nil
						}
					}
				}
			}
		}
	}
	return v
}

func (b *builder) vetPackage(pkg *Package, apkg *ast.Package) {
	errors := make(map[string]token.Pos)
	for _, file := range apkg.Files {
		for _, is := range file.Imports {
			importPath, _ := strconv.Unquote(is.Path.Value)
			if !gosrc.IsValidPath(importPath) &&
				!strings.HasPrefix(importPath, "exp/") &&
				!strings.HasPrefix(importPath, "appengine") {
				errors[fmt.Sprintf("Unrecognized import path %q", importPath)] = is.Pos()
			}
		}
		v := vetVisitor{errors: errors}
		ast.Walk(&v, file)
	}
	for message, pos := range errors {
		pkg.Errors = append(pkg.Errors,
			fmt.Sprintf("%s (%s)", message, b.fset.Position(pos)))
	}
}
