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

// Package talksapp implements the go-talks.appspot.com server.
package talksapp

import (
	"appengine"
	"appengine/memcache"
	"appengine/urlfetch"
	"bytes"
	"code.google.com/p/go.talks/pkg/present"
	"errors"
	"fmt"
	"github.com/garyburd/gosrc"
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

var (
	presentTemplates = map[string]*template.Template{
		".article": parsePresentTemplate("article.tmpl"),
		".slide":   parsePresentTemplate("slides.tmpl"),
	}
	homeArticle  = loadHomeArticle()
	contactEmail = "unknown@example.com"
)

func init() {
	http.Handle("/", handlerFunc(serveRoot))
	http.Handle("/compile", handlerFunc(serveCompile))
	http.Handle("/bot.html", handlerFunc(serveBot))
	present.PlayEnabled = true
}

func parsePresentTemplate(name string) *template.Template {
	t := present.Template()
	if _, err := t.ParseFiles("present/templates/"+name, "present/templates/action.tmpl"); err != nil {
		panic(err)
	}
	t = t.Lookup("root")
	if t == nil {
		panic("root template not found for " + name)
	}
	return t
}

func loadHomeArticle() []byte {
	const fname = "assets/home.article"
	f, err := os.Open(fname)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	doc, err := present.Parse(f, fname, 0)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err := renderPresentation(&buf, fname, doc); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func renderPresentation(w io.Writer, fname string, doc *present.Doc) error {
	t := presentTemplates[path.Ext(fname)]
	if t == nil {
		return errors.New("unknown template extension")
	}
	data := struct {
		*present.Doc
		Template    *template.Template
		PlayEnabled bool
	}{
		doc,
		t,
		true,
	}
	return t.Execute(w, &data)
}

type presFileNotFoundError string

func (s presFileNotFoundError) Error() string { return fmt.Sprintf("File %s not found.", string(s)) }

func writeHTMLHeader(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
}

func writeTextHeader(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
}

var setupOnce sync.Once

func setup(r *http.Request) {
	c := appengine.NewContext(r)
	gosrc.SetUserAgent(fmt.Sprintf("%s (+http://%s/bot.html)", appengine.AppID(c), r.Host))
}

type handlerFunc func(http.ResponseWriter, *http.Request) error

func (f handlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setupOnce.Do(func() { setup(r) })
	c := appengine.NewContext(r)
	err := f(w, r)
	if err == nil {
		return
	} else if gosrc.IsNotFound(err) {
		writeTextHeader(w, 400)
		io.WriteString(w, "Not Found.")
	} else if e, ok := err.(*gosrc.RemoteError); ok {
		writeTextHeader(w, 500)
		fmt.Fprintf(w, "Error accessing %s.", e.Host)
	} else if e, ok := err.(presFileNotFoundError); ok {
		writeTextHeader(w, 200)
		io.WriteString(w, e.Error())
	} else if err != nil {
		writeTextHeader(w, 500)
		io.WriteString(w, "Internal server error.")
		c.Errorf("Internal error %v", err)
	}
}

func serveRoot(w http.ResponseWriter, r *http.Request) error {
	switch {
	case r.Method != "GET":
		writeTextHeader(w, 405)
		_, err := io.WriteString(w, "Method not supported.")
		return err
	case r.URL.Path == "/":
		writeHTMLHeader(w, 200)
		_, err := w.Write(homeArticle)
		return err
	default:
		return servePresentation(w, r)
	}
}

func servePresentation(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	importPath := r.URL.Path[1:]

	item, err := memcache.Get(c, importPath)
	if err == nil {
		writeHTMLHeader(w, 200)
		w.Write(item.Value)
		return nil
	} else if err != memcache.ErrCacheMiss {
		return err
	}

	c.Infof("Fetching presentation %s.", importPath)
	pres, err := gosrc.GetPresentation(urlfetch.Client(c), importPath)
	if err != nil {
		return err
	}

	ctx := &present.Context{
		ReadFile: func(name string) ([]byte, error) {
			if p, ok := pres.Files[name]; ok {
				return p, nil
			}
			return nil, presFileNotFoundError(name)
		},
	}

	doc, err := ctx.Parse(bytes.NewReader(pres.Files[pres.Filename]), pres.Filename, 0)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := renderPresentation(&buf, importPath, doc); err != nil {
		return err
	}

	if err := memcache.Add(c, &memcache.Item{
		Key:        importPath,
		Value:      buf.Bytes(),
		Expiration: time.Hour,
	}); err != nil {
		return err
	}

	writeHTMLHeader(w, 200)
	_, err = w.Write(buf.Bytes())
	return err
}

func serveCompile(w http.ResponseWriter, r *http.Request) error {
	client := urlfetch.Client(appengine.NewContext(r))
	if err := r.ParseForm(); err != nil {
		return err
	}
	resp, err := client.PostForm("http://play.golang.org/compile", r.Form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	_, err = io.Copy(w, resp.Body)
	return err
}

func serveBot(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	writeTextHeader(w, 200)
	_, err := fmt.Fprintf(w, "Contact %s for help with the %s bot.", contactEmail, appengine.AppID(c))
	return err
}
