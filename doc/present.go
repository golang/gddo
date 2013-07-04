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
	"regexp"
	"time"
)

type Presentation struct {
	Filename string
	Files    map[string][]byte
	Updated  time.Time
}

type presBuilder struct {
	filename   string
	data       []byte
	resolveURL func(fname string) string
	fetch      func(srcs []*source) error
}

var assetPat = regexp.MustCompile(`(?m)^\.(play|code|image|iframe|html)\s+(\S+)`)

func (b *presBuilder) build() (*Presentation, error) {
	var data []byte
	var files []*source
	i := 0
	for _, m := range assetPat.FindAllSubmatchIndex(b.data, -1) {
		name := string(b.data[m[4]:m[5]])
		switch string(b.data[m[2]:m[3]]) {
		case "iframe", "image":
			data = append(data, b.data[i:m[4]]...)
			data = append(data, b.resolveURL(name)...)
		case "html":
			// TODO: sanitize and fix relative URLs in HTML.
			data = append(data, "\ntalks.godoc.org does not support .html\n"...)
		case "play", "code":
			data = append(data, b.data[i:m[5]]...)
			found := false
			for _, f := range files {
				if f.name == name {
					found = true
					break
				}
			}
			if !found {
				files = append(files, &source{name: name})
			}
		default:
			panic("unreachable")
		}
		i = m[5]
	}
	data = append(data, b.data[i:]...)
	if err := b.fetch(files); err != nil {
		return nil, err
	}
	pres := &Presentation{
		Updated:  time.Now().UTC(),
		Filename: b.filename,
		Files:    map[string][]byte{b.filename: data},
	}
	for _, f := range files {
		pres.Files[f.name] = f.data
	}
	return pres, nil
}
