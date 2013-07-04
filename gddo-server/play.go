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

package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/indigo/web"
)

func findExamples(pdoc *doc.Package, export, method string) []*doc.Example {
	if "package" == export {
		return pdoc.Examples
	}
	for _, f := range pdoc.Funcs {
		if f.Name == export {
			return f.Examples
		}
	}
	for _, t := range pdoc.Types {
		for _, f := range t.Funcs {
			if f.Name == export {
				return f.Examples
			}
		}
		if t.Name == export {
			if method == "" {
				return t.Examples
			}
			for _, m := range t.Methods {
				if method == m.Name {
					return m.Examples
				}
			}
			return nil
		}
	}
	return nil
}

func findExample(pdoc *doc.Package, export, method, name string) *doc.Example {
	for _, e := range findExamples(pdoc, export, method) {
		if name == e.Name {
			return e
		}
	}
	return nil
}

func playURL(pdoc *doc.Package, export string, name string) (string, error) {
	var method string
	if i := strings.Index(export, "-"); i > 0 {
		method = export[i+1:]
		export = export[:i]
	}
	if e := findExample(pdoc, export, method, name); e != nil && e.Play != "" {
		resp, err := httpClient.Post("http://play.golang.org/share", "text/plain", strings.NewReader(e.Play))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		p, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("http://play.golang.org/p/%s", p), nil
	}
	return "", &web.Error{Status: web.StatusNotFound}
}

func readPlayScript(dir string) (script []byte, err error) {
	for _, name := range []string{"jquery.js", "playground.js", "play.js"} {
		p, err := ioutil.ReadFile(filepath.Join(dir, "js", name))
		if err != nil {
			return nil, err
		}
		script = append(script, p...)
	}
	return
}
