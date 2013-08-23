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

// Command gddo-server is the GoPkgDoc server.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go.talks/pkg/present"
	"github.com/garyburd/gddo/database"
	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/indigo/server"
	"github.com/garyburd/indigo/web"
)

var errUpdateTimeout = errors.New("refresh timeout")

const (
	humanRequest = iota
	robotRequest
	queryRequest
	refreshRequest
)

type crawlResult struct {
	pdoc *doc.Package
	err  error
}

// getDoc gets the package documentation from the database or from the version
// control system as needed.
func getDoc(path string, requestType int) (*doc.Package, []database.Package, error) {
	if path == "-" {
		// A hack in the database package uses the path "-" to represent the
		// next document to crawl. Block "-" here so that requests to /- always
		// return not found.
		return nil, nil, nil
	}

	pdoc, pkgs, nextCrawl, err := db.Get(path)
	if err != nil {
		return nil, nil, err
	}

	needsCrawl := false
	switch requestType {
	case queryRequest:
		needsCrawl = nextCrawl.IsZero() && len(pkgs) == 0
	case humanRequest:
		needsCrawl = nextCrawl.Before(time.Now())
	case robotRequest:
		needsCrawl = nextCrawl.IsZero() && len(pkgs) > 0
	}

	if needsCrawl {
		c := make(chan crawlResult, 1)
		go func() {
			pdoc, err := crawlDoc("web  ", path, pdoc, len(pkgs) > 0, nextCrawl)
			c <- crawlResult{pdoc, err}
		}()
		var err error
		timeout := *getTimeout
		if pdoc == nil {
			timeout = *firstGetTimeout
		}
		select {
		case rr := <-c:
			if rr.err == nil {
				pdoc = rr.pdoc
			}
			err = rr.err
		case <-time.After(timeout):
			err = errUpdateTimeout
		}
		if err != nil {
			if pdoc != nil {
				log.Printf("Serving %q from database after error: %v", path, err)
				err = nil
			} else if err == errUpdateTimeout {
				// Handle timeout on packages never seeen before as not found.
				log.Printf("Serving %q as not found after timeout", path)
				err = &web.Error{Status: web.StatusNotFound}
			}
		}
	}
	return pdoc, pkgs, err
}

func templateExt(req *web.Request) string {
	if web.NegotiateContentType(req, []string{"text/html", "text/plain"}, "text/html") == "text/plain" {
		return ".txt"
	}
	return ".html"
}

var (
	robotPat = regexp.MustCompile(`(:?\+https?://)|(?:\Wbot\W)`)
)

func isRobot(req *web.Request) bool {
	return *robot || robotPat.MatchString(req.Header.Get(web.HeaderUserAgent))
}

func popularLinkReferral(req *web.Request) bool {
	u := url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host, Path: "/"}
	return req.Header.Get("Referer") == u.String()
}

func isView(req *web.Request, key string) bool {
	rq := req.URL.RawQuery
	return strings.HasPrefix(rq, key) &&
		(len(rq) == len(key) || rq[len(key)] == '=' || rq[len(key)] == '&')
}

func servePackage(resp web.Response, req *web.Request) error {
	p := path.Clean(req.URL.Path)
	if strings.HasPrefix(p, "/pkg/") {
		p = p[len("/pkg"):]
	}
	if p != req.URL.Path {
		return web.Redirect(resp, req, p, 301, nil)
	}

	requestType := humanRequest
	if isRobot(req) {
		requestType = robotRequest
	}

	importPath := req.RouteVars["path"]
	pdoc, pkgs, err := getDoc(importPath, requestType)
	if err != nil {
		return err
	}

	if pdoc == nil {
		if len(pkgs) == 0 {
			return &web.Error{Status: web.StatusNotFound}
		}
		pdocChild, _, _, err := db.Get(pkgs[0].Path)
		if err != nil {
			return err
		}
		pdoc = &doc.Package{
			ProjectName: pdocChild.ProjectName,
			ProjectRoot: pdocChild.ProjectRoot,
			ProjectURL:  pdocChild.ProjectURL,
			ImportPath:  importPath,
		}
	}

	switch {
	case len(req.Form) == 0:
		if requestType == humanRequest &&
			pdoc.Name != "" && // not a directory
			pdoc.ProjectRoot != "" && // not a standard package
			!pdoc.IsCmd &&
			len(pdoc.Errors) == 0 &&
			!popularLinkReferral(req) {
			if err := db.IncrementPopularScore(pdoc.ImportPath); err != nil {
				log.Print("ERROR db.IncrementPopularScore(%s): %v", pdoc.ImportPath, err)
			}
		}

		importerCount, err := db.ImporterCount(importPath)
		if err != nil {
			return err
		}

		template := "dir"
		switch {
		case pdoc.IsCmd:
			template = "cmd"
		case pdoc.Name != "":
			template = "pkg"
		}
		template += templateExt(req)

		if srcFiles[importPath+"/_sourceMap"] != nil {
			for _, f := range pdoc.Files {
				if srcFiles[importPath+"/"+f.Name] != nil {
					f.URL = fmt.Sprintf("/%s?file=%s", importPath, f.Name)
					pdoc.LineFmt = "%s#L%d"
				}
			}
		}

		return executeTemplate(resp, template, web.StatusOK, nil, map[string]interface{}{
			"pkgs":          pkgs,
			"pdoc":          newTDoc(pdoc),
			"importerCount": importerCount,
		})
	case isView(req, "imports"):
		if pdoc.Name == "" {
			break
		}
		pkgs, err = db.Packages(pdoc.Imports)
		if err != nil {
			return err
		}
		return executeTemplate(resp, "imports.html", web.StatusOK, nil, map[string]interface{}{
			"pkgs": pkgs,
			"pdoc": newTDoc(pdoc),
		})
	case isView(req, "redir"):
		if srcFiles == nil {
			break
		}
		f := srcFiles[importPath+"/_sourceMap"]
		if f == nil {
			break
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		var sourceMap map[string]string
		if err := gob.NewDecoder(r).Decode(&sourceMap); err != nil {
			return err
		}
		id := req.Form.Get("redir")
		fname := sourceMap[id]
		if fname == "" {
			break
		}
		return web.Redirect(resp, req, fmt.Sprintf("?file=%s#%s", fname, id), 301, nil)
	case isView(req, "file"):
		if srcFiles == nil {
			break
		}
		fname := req.Form.Get("file")
		f := srcFiles[importPath+"/"+fname]
		if f == nil {
			break
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		src := make([]byte, f.UncompressedSize64)
		if n, err := io.ReadFull(r, src); err != nil {
			return err
		} else {
			src = src[:n]
		}
		var url string
		for _, f := range pdoc.Files {
			if f.Name == fname {
				url = f.URL
			}
		}
		return executeTemplate(resp, "file.html", web.StatusOK, nil, map[string]interface{}{
			"fname": fname,
			"url":   url,
			"src":   template.HTML(src),
			"pdoc":  newTDoc(pdoc),
		})
	case isView(req, "importers"):
		if pdoc.Name == "" {
			break
		}
		pkgs, err = db.Importers(importPath)
		if err != nil {
			return err
		}
		return executeTemplate(resp, "importers.html", web.StatusOK, nil, map[string]interface{}{
			"pkgs": pkgs,
			"pdoc": newTDoc(pdoc),
		})
	case isView(req, "import-graph"):
		if pdoc.Name == "" {
			break
		}
		hide := req.Form.Get("hide") == "1"
		pkgs, edges, err := db.ImportGraph(pdoc, hide)
		if err != nil {
			return err
		}
		b, err := renderGraph(pdoc, pkgs, edges)
		if err != nil {
			return err
		}
		return executeTemplate(resp, "graph.html", web.StatusOK, nil, map[string]interface{}{
			"svg":  template.HTML(b),
			"pdoc": newTDoc(pdoc),
			"hide": hide,
		})
	case isView(req, "play"):
		u, err := playURL(pdoc, req.Form.Get("play"))
		if err != nil {
			return err
		}
		return web.Redirect(resp, req, u, 301, nil)
	case req.Form.Get("view") != "":
		// Redirect deprecated view= queries.
		var q string
		switch view := req.Form.Get("view"); view {
		case "imports", "importers":
			q = view
		case "import-graph":
			if req.Form.Get("hide") == "1" {
				q = "import-graph&hide=1"
			} else {
				q = "import-graph"
			}
		}
		if q != "" {
			u := *req.URL
			u.RawQuery = q
			return web.Redirect(resp, req, u.String(), 301, nil)
		}
	}
	return &web.Error{Status: web.StatusNotFound}
}

func serveRefresh(resp web.Response, req *web.Request) error {
	path := req.Form.Get("path")
	_, pkgs, _, err := db.Get(path)
	if err != nil {
		return err
	}
	c := make(chan error, 1)
	go func() {
		_, err := crawlDoc("rfrsh", path, nil, len(pkgs) > 0, time.Time{})
		c <- err
	}()
	select {
	case err = <-c:
	case <-time.After(*getTimeout):
		err = errUpdateTimeout
	}
	if err != nil {
		return err
	}
	return web.Redirect(resp, req, "/"+path, 302, nil)
}

func serveGoIndex(resp web.Response, req *web.Request) error {
	pkgs, err := db.GoIndex()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "std.html", web.StatusOK, nil, map[string]interface{}{
		"pkgs": pkgs,
	})
}

func serveGoSubrepoIndex(resp web.Response, req *web.Request) error {
	pkgs, err := db.GoSubrepoIndex()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "subrepo.html", web.StatusOK, nil, map[string]interface{}{
		"pkgs": pkgs,
	})
}

func serveIndex(resp web.Response, req *web.Request) error {
	pkgs, err := db.Index()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "index.html", web.StatusOK, nil, map[string]interface{}{
		"pkgs": pkgs,
	})
}

type byPath struct {
	pkgs []database.Package
	rank []int
}

func (bp *byPath) Len() int           { return len(bp.pkgs) }
func (bp *byPath) Less(i, j int) bool { return bp.pkgs[i].Path < bp.pkgs[j].Path }
func (bp *byPath) Swap(i, j int) {
	bp.pkgs[i], bp.pkgs[j] = bp.pkgs[j], bp.pkgs[i]
	bp.rank[i], bp.rank[j] = bp.rank[j], bp.rank[i]
}

type byRank struct {
	pkgs []database.Package
	rank []int
}

func (br *byRank) Len() int           { return len(br.pkgs) }
func (br *byRank) Less(i, j int) bool { return br.rank[i] < br.rank[j] }
func (br *byRank) Swap(i, j int) {
	br.pkgs[i], br.pkgs[j] = br.pkgs[j], br.pkgs[i]
	br.rank[i], br.rank[j] = br.rank[j], br.rank[i]
}

func popular() ([]database.Package, error) {
	const n = 25

	pkgs, err := db.Popular(2 * n)
	if err != nil {
		return nil, err
	}

	rank := make([]int, len(pkgs))
	for i := range pkgs {
		rank[i] = i
	}

	sort.Sort(&byPath{pkgs, rank})

	j := 0
	prev := "."
	for i, pkg := range pkgs {
		if strings.HasPrefix(pkg.Path, prev) {
			if rank[j-1] < rank[i] {
				rank[j-1] = rank[i]
			}
			continue
		}
		prev = pkg.Path + "/"
		pkgs[j] = pkg
		rank[j] = rank[i]
		j += 1
	}
	pkgs = pkgs[:j]

	sort.Sort(&byRank{pkgs, rank})

	if len(pkgs) > n {
		pkgs = pkgs[:n]
	}

	sort.Sort(&byPath{pkgs, rank})

	return pkgs, nil
}

func serveHome(resp web.Response, req *web.Request) error {

	q := strings.TrimSpace(req.Form.Get("q"))
	if q == "" {
		pkgs, err := popular()
		if err != nil {
			return err
		}

		return executeTemplate(resp, "home"+templateExt(req), web.StatusOK, nil,
			map[string]interface{}{"Popular": pkgs})
	}

	if path, ok := isBrowseURL(q); ok {
		q = path
	}

	if doc.IsValidRemotePath(q) {
		pdoc, pkgs, err := getDoc(q, queryRequest)
		if err == nil && (pdoc != nil || len(pkgs) > 0) {
			return web.Redirect(resp, req, "/"+q, 302, nil)
		}
	}

	pkgs, err := db.Query(q)
	if err != nil {
		return err
	}

	return executeTemplate(resp, "results"+templateExt(req), web.StatusOK, nil,
		map[string]interface{}{"q": q, "pkgs": pkgs})
}

func serveAbout(resp web.Response, req *web.Request) error {
	return executeTemplate(resp, "about.html", web.StatusOK, nil,
		map[string]interface{}{"Host": req.URL.Host})
}

func serveBot(resp web.Response, req *web.Request) error {
	return executeTemplate(resp, "bot.html", web.StatusOK, nil, nil)
}

func serveOpenSearchDescription(resp web.Response, req *web.Request) error {
	return executeTemplate(resp, "opensearch.xml", web.StatusOK, nil, req.URL.Host)
}

func serveTypeahead(resp web.Response, req *web.Request) error {
	pkgs, err := db.Popular(1000)
	if err != nil {
		return err
	}
	items := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		items[i] = pkg.Path
	}
	data := map[string]interface{}{"items": items}
	w := resp.Start(web.StatusOK, web.Header{web.HeaderContentType: {"application/json; charset=utf-8"}})
	return json.NewEncoder(w).Encode(data)
}

func logError(req *web.Request, err error, r interface{}) {
	if err != nil {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Error serving %s: %v\n", req.URL, err)
		if r != nil {
			fmt.Fprintln(&buf, r)
			buf.Write(debug.Stack())
		}
		log.Print(buf.String())
	}
}

func renderPresentation(resp web.Response, fname string, doc *present.Doc) error {
	t := presentTemplates[path.Ext(fname)]
	data := struct {
		*present.Doc
		Template    *template.Template
		PlayEnabled bool
	}{
		doc,
		t,
		true,
	}

	return t.Execute(
		resp.Start(web.StatusOK, web.Header{web.HeaderContentType: {"text/html; charset=utf8"}}),
		&data)
}

func servePresentHome(resp web.Response, req *web.Request) error {
	fname := filepath.Join(*assetsDir, "presentHome.article")
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	doc, err := present.Parse(f, fname, 0)
	if err != nil {
		return err
	}
	return renderPresentation(resp, fname, doc)
}

var (
	presMu        sync.Mutex
	presentations = map[string]*doc.Presentation{}
)

func servePresentation(resp web.Response, req *web.Request) error {
	if p := path.Clean(req.URL.Path); p != req.URL.Path {
		return web.Redirect(resp, req, p, 301, nil)
	}

	presMu.Lock()
	for p, pres := range presentations {
		if time.Since(pres.Updated) > 15*time.Minute {
			delete(presentations, p)
		}
	}
	p := req.RouteVars["path"]
	pres := presentations[p]
	presMu.Unlock()

	if pres == nil {
		var err error
		log.Println("Fetch presentation ", p)
		pres, err = doc.GetPresentation(httpClient, p)
		if err != nil {
			return err
		}
		presMu.Lock()
		presentations[p] = pres
		presMu.Unlock()
	}
	ctx := &present.Context{
		ReadFile: func(name string) ([]byte, error) {
			if p, ok := pres.Files[name]; ok {
				return p, nil
			}
			return nil, fmt.Errorf("pres file not found %s", name)
		},
	}
	doc, err := ctx.Parse(bytes.NewReader(pres.Files[pres.Filename]), pres.Filename, 0)
	if err != nil {
		return err
	}
	return renderPresentation(resp, p, doc)
}

func serveCompile(resp web.Response, req *web.Request) error {
	r, err := http.PostForm("http://golang.org/compile", req.Form)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.Copy(
		resp.Start(web.StatusOK, web.Header{web.HeaderContentType: r.Header[web.HeaderContentType]}),
		r.Body)
	return err
}

func serveAPISearch(resp web.Response, req *web.Request) error {
	q := strings.TrimSpace(req.Form.Get("q"))
	pkgs, err := db.Query(q)
	if err != nil {
		return err
	}

	var data struct {
		Results []database.Package `json:"results"`
	}
	data.Results = pkgs
	w := resp.Start(web.StatusOK, web.Header{web.HeaderContentType: {"application/json; charset=utf-8"}})
	return json.NewEncoder(w).Encode(&data)
}

func serveAPIPackages(resp web.Response, req *web.Request) error {
	pkgs, err := db.AllPackages()
	if err != nil {
		return err
	}
	var data struct {
		Results []database.Package `json:"results"`
	}
	data.Results = pkgs
	w := resp.Start(web.StatusOK, web.Header{web.HeaderContentType: {"application/json; charset=utf-8"}})
	return json.NewEncoder(w).Encode(&data)
}

func serveAPIImporters(resp web.Response, req *web.Request) error {
	pkgs, err := db.Importers(req.RouteVars["path"])
	if err != nil {
		return err
	}
	var data struct {
		Results []database.Package `json:"results"`
	}
	data.Results = pkgs
	w := resp.Start(web.StatusOK, web.Header{web.HeaderContentType: {"application/json; charset=utf-8"}})
	return json.NewEncoder(w).Encode(&data)
}

func handleError(resp web.Response, req *web.Request, status int, err error, r interface{}) {
	logError(req, err, r)
	switch status {
	case 0:
		// nothing to do
	case web.StatusNotFound:
		executeTemplate(resp, "notfound"+templateExt(req), status, nil, nil)
	default:
		s := web.StatusText(status)
		if err == errUpdateTimeout {
			s = "Timeout getting package files from the version control system."
		} else if e, ok := err.(*doc.RemoteError); ok {
			s = "Error getting package files from " + e.Host + "."
		}
		w := resp.Start(web.StatusInternalServerError, web.Header{web.HeaderContentType: {"text/plan; charset=utf-8"}})
		io.WriteString(w, s)
	}
}

func handlePresentError(resp web.Response, req *web.Request, status int, err error, r interface{}) {
	logError(req, err, r)
	switch status {
	case 0:
		// nothing to do
	default:
		s := web.StatusText(status)
		if doc.IsNotFound(err) {
			s = web.StatusText(web.StatusNotFound)
			status = web.StatusNotFound
		} else if err == errUpdateTimeout {
			s = "Timeout getting package files from the version control system."
		} else if e, ok := err.(*doc.RemoteError); ok {
			s = "Error getting package files from " + e.Host + "."
		}
		w := resp.Start(status, web.Header{web.HeaderContentType: {"text/plan; charset=utf-8"}})
		io.WriteString(w, s)
	}
}

func handleAPIError(resp web.Response, req *web.Request, status int, err error, r interface{}) {
	logError(req, err, r)
	switch status {
	case 0:
		// nothing to do
	default:
		var data struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		data.Error.Message = web.StatusText(status)
		w := resp.Start(status, web.Header{web.HeaderContentType: {"application/json; charset=utf-8"}})
		json.NewEncoder(w).Encode(&data)
	}
}

func defaultBase(path string) string {
	p, err := build.Default.Import(path, "", build.FindOnly)
	if err != nil {
		return "."
	}
	return p.Dir
}

var (
	db              *database.Database
	robot           = flag.Bool("robot", false, "Robot mode")
	assetsDir       = flag.String("assets", filepath.Join(defaultBase("github.com/garyburd/gddo/gddo-server"), "assets"), "Base directory for templates and static files.")
	gzAssetsDir     = flag.String("gzassets", "", "Base directory for compressed static files.")
	presentDir      = flag.String("present", defaultBase("code.google.com/p/go.talks/present"), "Base directory for templates and static files.")
	getTimeout      = flag.Duration("get_timeout", 8*time.Second, "Time to wait for package update from the VCS.")
	firstGetTimeout = flag.Duration("first_get_timeout", 5*time.Second, "Time to wait for first fetch of package from the VCS.")
	maxAge          = flag.Duration("max_age", 24*time.Hour, "Update package documents older than this age.")
	httpAddr        = flag.String("http", ":8080", "Listen for HTTP connections on this address")
	secretsPath     = flag.String("secrets", "secrets.json", "Path to file containing application ids and credentials for other services.")
	redirGoTalks    = flag.Bool("redirGoTalks", true, "Redirect paths with prefix 'code.google.com/p/go.talks/' to talks.golang.org")
	srcZip          = flag.String("srcZip", "", "")

	secrets struct {
		// HTTP user agent for outbound requests
		UserAgent string

		// GitHub API Credentials
		GitHubId     string
		GitHubSecret string

		// Google Analytics account for tracking codes.
		GAAccount string
	}

	srcFiles = make(map[string]*zip.File)
)

func readSecrets() error {
	b, err := ioutil.ReadFile(*secretsPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &secrets); err != nil {
		return err
	}
	if secrets.UserAgent != "" {
		doc.SetUserAgent(secrets.UserAgent)
	}
	if secrets.GitHubId != "" {
		doc.SetGitHubCredentials(secrets.GitHubId, secrets.GitHubSecret)
	} else {
		log.Printf("GitHub credentials not set in %q.", *secretsPath)
	}
	return nil
}

var cacheBusters = map[string]string{}

func dataHandler(cacheBusterKey, contentType, dir string, names ...string) web.Handler {
	var data []byte
	for _, name := range names {
		p, err := ioutil.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			log.Fatal(err)
		}
		data = append(data, p...)
	}

	h := md5.New()
	h.Write(data)
	cacheBusters[cacheBusterKey] = fmt.Sprintf("%x", h.Sum(nil))

	return web.DataHandler(data, web.Header{web.HeaderContentType: {contentType}})
}

func main() {
	flag.Parse()
	log.Printf("Starting server, os.Args=%s", strings.Join(os.Args, " "))
	if err := readSecrets(); err != nil {
		log.Fatal(err)
	}

	if *srcZip != "" {
		r, err := zip.OpenReader(*srcZip)
		if err != nil {
			log.Fatal(err)
		}
		for _, f := range r.File {
			if strings.HasPrefix(f.Name, "root/") {
				srcFiles[f.Name[len("root/"):]] = f
			}
		}
	}

	if err := parseHTMLTemplates([][]string{
		{"about.html", "common.html", "layout.html"},
		{"bot.html", "common.html", "layout.html"},
		{"cmd.html", "common.html", "layout.html"},
		{"dir.html", "common.html", "layout.html"},
		{"home.html", "common.html", "layout.html"},
		{"importers.html", "common.html", "layout.html"},
		{"imports.html", "common.html", "layout.html"},
		{"file.html", "common.html", "layout.html"},
		{"index.html", "common.html", "layout.html"},
		{"notfound.html", "common.html", "layout.html"},
		{"pkg.html", "common.html", "layout.html"},
		{"results.html", "common.html", "layout.html"},
		{"std.html", "common.html", "layout.html"},
		{"subrepo.html", "common.html", "layout.html"},
		{"graph.html", "common.html"},
	}); err != nil {
		log.Fatal(err)
	}

	if err := parseTextTemplates([][]string{
		{"cmd.txt", "common.txt"},
		{"dir.txt", "common.txt"},
		{"home.txt", "common.txt"},
		{"notfound.txt", "common.txt"},
		{"pkg.txt", "common.txt"},
		{"results.txt", "common.txt"},
		{"opensearch.xml"},
	}); err != nil {
		log.Fatal(err)
	}

	if err := parsePresentTemplates([][]string{
		{".article", "article.tmpl", "action.tmpl"},
		{".slide", "slides.tmpl", "action.tmpl"},
	}); err != nil {
		log.Fatal(err)
	}

	present.PlayEnabled = true

	var err error
	db, err = database.New()
	if err != nil {
		log.Fatal(err)
	}

	go runBackgroundTasks()

	staticConfig := &web.StaticConfig{
		Header:      web.Header{web.HeaderCacheControl: {"public, max-age=3600"}},
		Directory:   *assetsDir,
		GzDirectory: *gzAssetsDir,
	}
	presentStaticConfig := &web.StaticConfig{
		Header:    web.Header{web.HeaderCacheControl: {"public, max-age=3600"}},
		Directory: *presentDir,
	}

	h := web.NewHostRouter()

	r := web.NewRouter()
	r.Add("/").GetFunc(servePresentHome)
	r.Add("/compile").PostFunc(serveCompile)
	r.Add("/favicon.ico").Get(staticConfig.FileHandler("favicon.ico"))
	r.Add("/google3d2f3cd4cc2bb44b.html").Get(staticConfig.FileHandler("google3d2f3cd4cc2bb44b.html"))
	r.Add("/humans.txt").Get(staticConfig.FileHandler("humans.txt"))
	r.Add("/play.js").Get(dataHandler("play.js", "text/javascript", *presentDir, "js/jquery.js", "js/playground.js", "js/play.js"))
	r.Add("/robots.txt").Get(staticConfig.FileHandler("presentRobots.txt"))
	r.Add("/static/<path:.*>").Get(presentStaticConfig.DirectoryHandler("static"))
	if *redirGoTalks {
		r.Add("/code.google.com/p/go.talks/<path:.+>").GetFunc(func(resp web.Response, req *web.Request) error {
			return web.Redirect(resp, req, "http://talks.golang.org/"+req.RouteVars["path"], 301, nil)
		})
	}
	r.Add("/<path:.+>").GetFunc(servePresentation)

	h.Add("talks.<:.*>", web.ErrorHandler(handlePresentError, web.FormAndCookieHandler(6000, false, r)))

	r = web.NewRouter()
	r.Add("/favicon.ico").Get(staticConfig.FileHandler("favicon.ico"))
	r.Add("/google3d2f3cd4cc2bb44b.html").Get(staticConfig.FileHandler("google3d2f3cd4cc2bb44b.html"))
	r.Add("/humans.txt").Get(staticConfig.FileHandler("humans.txt"))
	r.Add("/robots.txt").Get(staticConfig.FileHandler("presentRobots.txt"))
	r.Add("/search").GetFunc(serveAPISearch)
	r.Add("/packages").GetFunc(serveAPIPackages)
	r.Add("/importers/<path:.+>").GetFunc(serveAPIImporters)

	h.Add("api.<:.*>", web.ErrorHandler(handleAPIError, web.FormAndCookieHandler(6000, false, r)))

	r = web.NewRouter()
	r.Add("/-/site.js").Get(dataHandler("site.js", "text/javascript", *assetsDir,
		"third_party/jquery.timeago.js",
		"third_party/typeahead.min.js",
		"third_party/bootstrap/js/bootstrap.min.js",
		"site.js"))
	r.Add("/-/site.css").Get(dataHandler("site.css", "text/css", *assetsDir,
		"third_party/bootstrap/css/bootstrap.min.css", "site.css"))
	r.Add("/").GetFunc(serveHome)
	r.Add("/-/about").GetFunc(serveAbout)
	r.Add("/-/bot").GetFunc(serveBot)
	r.Add("/-/opensearch.xml").GetFunc(serveOpenSearchDescription)
	r.Add("/-/typeahead").GetFunc(serveTypeahead)
	r.Add("/-/go").GetFunc(serveGoIndex)
	r.Add("/-/subrepo").GetFunc(serveGoSubrepoIndex)
	r.Add("/-/index").GetFunc(serveIndex)
	r.Add("/-/refresh").PostFunc(serveRefresh)
	r.Add("/-/static/<path:.*>").Get(staticConfig.DirectoryHandler("static"))
	r.Add("/a/index").Get(web.RedirectHandler("/-/index", 301))
	r.Add("/about").Get(web.RedirectHandler("/-/about", 301))
	r.Add("/favicon.ico").Get(staticConfig.FileHandler("favicon.ico"))
	r.Add("/google3d2f3cd4cc2bb44b.html").Get(staticConfig.FileHandler("google3d2f3cd4cc2bb44b.html"))
	r.Add("/humans.txt").Get(staticConfig.FileHandler("humans.txt"))
	r.Add("/robots.txt").Get(staticConfig.FileHandler("robots.txt"))
	r.Add("/BingSiteAuth.xml").Get(staticConfig.FileHandler("BingSiteAuth.xml"))
	r.Add("/C").Get(web.RedirectHandler("http://golang.org/doc/articles/c_go_cgo.html", 301))
	r.Add("/<path:.+>").GetFunc(servePackage)

	h.Add("<:.*>", web.ErrorHandler(handleError, web.FormAndCookieHandler(1000, false, r)))

	listener, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		log.Fatal("Listen", err)
		return
	}
	defer listener.Close()
	s := &server.Server{Listener: listener, Handler: h} // add logger
	err = s.Serve()
	if err != nil {
		log.Fatal("Server", err)
	}
}
