// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

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
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/gddo/database"
	"github.com/garyburd/gddo/doc"
	"github.com/garyburd/gddo/httputil"
	"github.com/garyburd/gosrc"
)

const (
	jsonMIMEType = "application/json; charset=utf-8"
	textMIMEType = "text/plain; charset=utf-8"
	htmlMIMEType = "text/html; charset=utf-8"
)

var errUpdateTimeout = errors.New("refresh timeout")

type httpError struct {
	status int   // HTTP status code.
	err    error // Optional reason for the HTTP error.
}

func (err *httpError) Error() string {
	if err.err != nil {
		return fmt.Sprintf("status %d, reason %s", err.status, err.err.Error())
	}
	return fmt.Sprintf("Status %d", err.status)
}

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
				err = &httpError{status: http.StatusNotFound}
			}
		}
	}
	return pdoc, pkgs, err
}

func templateExt(req *http.Request) string {
	if httputil.NegotiateContentType(req, []string{"text/html", "text/plain"}, "text/html") == "text/plain" {
		return ".txt"
	}
	return ".html"
}

var (
	robotPat = regexp.MustCompile(`(:?\+https?://)|(?:\Wbot\W)|(?:^Python-urllib)|(?:^Go )|(?:^Java/)`)
)

func isRobot(req *http.Request) bool {
	if robotPat.MatchString(req.Header.Get("User-Agent")) {
		return true
	}
	host := httputil.StripPort(req.RemoteAddr)
	n, err := db.IncrementCounter(host, 1)
	if err != nil {
		log.Printf("error incrementing counter for %s,  %v\n", host, err)
		return false
	}
	if n > *robot {
		log.Printf("robot %.2f %s %s", n, host, req.Header.Get("User-Agent"))
		return true
	}
	return false
}

func popularLinkReferral(req *http.Request) bool {
	return strings.HasSuffix(req.Header.Get("Referer"), "//"+req.Host+"/")
}

func isView(req *http.Request, key string) bool {
	rq := req.URL.RawQuery
	return strings.HasPrefix(rq, key) &&
		(len(rq) == len(key) || rq[len(key)] == '=' || rq[len(key)] == '&')
}

// httpEtag returns the package entity tag used in HTTP transactions.
func httpEtag(pdoc *doc.Package, pkgs []database.Package, importerCount int) string {
	b := make([]byte, 0, 128)
	b = strconv.AppendInt(b, pdoc.Updated.Unix(), 16)
	b = append(b, 0)
	b = append(b, pdoc.Etag...)
	if importerCount >= 8 {
		importerCount = 8
	}
	b = append(b, 0)
	b = strconv.AppendInt(b, int64(importerCount), 16)
	for _, pkg := range pkgs {
		b = append(b, 0)
		b = append(b, pkg.Path...)
		b = append(b, 0)
		b = append(b, pkg.Synopsis...)
	}
	if *sidebarEnabled {
		b = append(b, "\000xsb"...)
	}
	h := md5.New()
	h.Write(b)
	b = h.Sum(b[:0])
	return fmt.Sprintf("\"%x\"", b)
}

func servePackage(resp http.ResponseWriter, req *http.Request) error {
	p := path.Clean(req.URL.Path)
	if strings.HasPrefix(p, "/pkg/") {
		p = p[len("/pkg"):]
	}
	if p != req.URL.Path {
		http.Redirect(resp, req, p, 301)
		return nil
	}

	if isView(req, "status.png") {
		statusImageHandler.ServeHTTP(resp, req)
		return nil
	}

	requestType := humanRequest
	if isRobot(req) {
		requestType = robotRequest
	}

	importPath := strings.TrimPrefix(req.URL.Path, "/")
	pdoc, pkgs, err := getDoc(importPath, requestType)
	if err != nil {
		return err
	}

	if pdoc == nil {
		if len(pkgs) == 0 {
			return &httpError{status: http.StatusNotFound}
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
		importerCount, err := db.ImporterCount(importPath)
		if err != nil {
			return err
		}

		etag := httpEtag(pdoc, pkgs, importerCount)
		status := http.StatusOK
		if req.Header.Get("If-None-Match") == etag {
			status = http.StatusNotModified
		}

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

		return executeTemplate(resp, template, status, http.Header{"Etag": {etag}}, map[string]interface{}{
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
		return executeTemplate(resp, "imports.html", http.StatusOK, nil, map[string]interface{}{
			"pkgs": pkgs,
			"pdoc": newTDoc(pdoc),
		})
	case isView(req, "tools"):
		proto := "http"
		if req.Host == "godoc.org" {
			proto = "https"
		}
		return executeTemplate(resp, "tools.html", http.StatusOK, nil, map[string]interface{}{
			"uri":  fmt.Sprintf("%s://%s/%s", proto, req.Host, importPath),
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
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		var sourceMap map[string]string
		if err := gob.NewDecoder(rc).Decode(&sourceMap); err != nil {
			return err
		}
		id := req.Form.Get("redir")
		fname := sourceMap[id]
		if fname == "" {
			break
		}
		http.Redirect(resp, req, fmt.Sprintf("?file=%s#%s", fname, id), 301)
		return nil
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
		return executeTemplate(resp, "file.html", http.StatusOK, nil, map[string]interface{}{
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
		template := "importers.html"
		if requestType == robotRequest {
			// Hide back links from robots.
			template = "importers_robot.html"
		}
		return executeTemplate(resp, template, http.StatusOK, nil, map[string]interface{}{
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
		return executeTemplate(resp, "graph.html", http.StatusOK, nil, map[string]interface{}{
			"svg":  template.HTML(b),
			"pdoc": newTDoc(pdoc),
			"hide": hide,
		})
	case isView(req, "play"):
		u, err := playURL(pdoc, req.Form.Get("play"))
		if err != nil {
			return err
		}
		http.Redirect(resp, req, u, 301)
		return nil
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
			http.Redirect(resp, req, u.String(), 301)
			return nil
		}
	}
	return &httpError{status: http.StatusNotFound}
}

func serveRefresh(resp http.ResponseWriter, req *http.Request) error {
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
	http.Redirect(resp, req, "/"+path, 302)
	return nil
}

func serveGoIndex(resp http.ResponseWriter, req *http.Request) error {
	pkgs, err := db.GoIndex()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "std.html", http.StatusOK, nil, map[string]interface{}{
		"pkgs": pkgs,
	})
}

func serveGoSubrepoIndex(resp http.ResponseWriter, req *http.Request) error {
	pkgs, err := db.GoSubrepoIndex()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "subrepo.html", http.StatusOK, nil, map[string]interface{}{
		"pkgs": pkgs,
	})
}

func serveIndex(resp http.ResponseWriter, req *http.Request) error {
	pkgs, err := db.Index()
	if err != nil {
		return err
	}
	return executeTemplate(resp, "index.html", http.StatusOK, nil, map[string]interface{}{
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

func serveHome(resp http.ResponseWriter, req *http.Request) error {
	if req.URL.Path != "/" {
		return servePackage(resp, req)
	}

	q := strings.TrimSpace(req.Form.Get("q"))
	if q == "" {
		pkgs, err := popular()
		if err != nil {
			return err
		}

		return executeTemplate(resp, "home"+templateExt(req), http.StatusOK, nil,
			map[string]interface{}{"Popular": pkgs})
	}

	if path, ok := isBrowseURL(q); ok {
		q = path
	}

	if gosrc.IsValidRemotePath(q) || (strings.Contains(q, "/") && gosrc.IsGoRepoPath(q)) {
		pdoc, pkgs, err := getDoc(q, queryRequest)
		if err == nil && (pdoc != nil || len(pkgs) > 0) {
			http.Redirect(resp, req, "/"+q, 302)
			return nil
		}
	}

	pkgs, err := db.Query(q)
	if err != nil {
		return err
	}

	return executeTemplate(resp, "results"+templateExt(req), http.StatusOK, nil,
		map[string]interface{}{"q": q, "pkgs": pkgs})
}

func serveAbout(resp http.ResponseWriter, req *http.Request) error {
	return executeTemplate(resp, "about.html", http.StatusOK, nil,
		map[string]interface{}{"Host": req.Host})
}

func serveBot(resp http.ResponseWriter, req *http.Request) error {
	return executeTemplate(resp, "bot.html", http.StatusOK, nil, nil)
}

func logError(req *http.Request, err error, rv interface{}) {
	if err != nil {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Error serving %s: %v\n", req.URL, err)
		if rv != nil {
			fmt.Fprintln(&buf, rv)
			buf.Write(debug.Stack())
		}
		log.Print(buf.String())
	}
}

func serveAPISearch(resp http.ResponseWriter, req *http.Request) error {
	q := strings.TrimSpace(req.Form.Get("q"))
	pkgs, err := db.Query(q)
	if err != nil {
		return err
	}

	var data struct {
		Results []database.Package `json:"results"`
	}
	data.Results = pkgs
	resp.Header().Set("Content-Type", jsonMIMEType)
	return json.NewEncoder(resp).Encode(&data)
}

func serveAPIPackages(resp http.ResponseWriter, req *http.Request) error {
	pkgs, err := db.AllPackages()
	if err != nil {
		return err
	}
	data := struct {
		Results []database.Package `json:"results"`
	}{
		pkgs,
	}
	resp.Header().Set("Content-Type", jsonMIMEType)
	return json.NewEncoder(resp).Encode(&data)
}

func serveAPIImporters(resp http.ResponseWriter, req *http.Request) error {
	importPath := strings.TrimPrefix(req.URL.Path, "/importers/")
	pkgs, err := db.Importers(importPath)
	if err != nil {
		return err
	}
	data := struct {
		Results []database.Package `json:"results"`
	}{
		pkgs,
	}
	resp.Header().Set("Content-Type", jsonMIMEType)
	return json.NewEncoder(resp).Encode(&data)
}

func serveAPIImports(resp http.ResponseWriter, req *http.Request) error {
	importPath := strings.TrimPrefix(req.URL.Path, "/imports/")
	pdoc, _, err := getDoc(importPath, robotRequest)
	if err != nil {
		return err
	}
	if pdoc == nil || pdoc.Name == "" {
		return &httpError{status: http.StatusNotFound}
	}
	imports, err := db.Packages(pdoc.Imports)
	if err != nil {
		return err
	}
	testImports, err := db.Packages(pdoc.TestImports)
	if err != nil {
		return err
	}
	data := struct {
		Imports     []database.Package `json:"imports"`
		TestImports []database.Package `json:"testImports"`
	}{
		imports,
		testImports,
	}
	resp.Header().Set("Content-Type", jsonMIMEType)
	return json.NewEncoder(resp).Encode(&data)
}

func serveAPIHome(resp http.ResponseWriter, req *http.Request) error {
	return &httpError{status: http.StatusNotFound}
}

func runHandler(resp http.ResponseWriter, req *http.Request,
	fn func(resp http.ResponseWriter, req *http.Request) error, errfn httputil.Error) {
	defer func() {
		if rv := recover(); rv != nil {
			err := errors.New("handler panic")
			logError(req, err, rv)
			errfn(resp, req, http.StatusInternalServerError, err)
		}
	}()

	if s := req.Header.Get("X-Real-Ip"); s != "" && httputil.StripPort(req.RemoteAddr) == "127.0.0.1" {
		req.RemoteAddr = s
	}

	req.Body = http.MaxBytesReader(resp, req.Body, 2048)
	req.ParseForm()
	var rb httputil.ResponseBuffer
	err := fn(&rb, req)
	if err == nil {
		rb.WriteTo(resp)
	} else if e, ok := err.(*httpError); ok {
		if e.status >= 500 {
			logError(req, err, nil)
		}
		errfn(resp, req, e.status, e.err)
	} else {
		logError(req, err, nil)
		errfn(resp, req, http.StatusInternalServerError, err)
	}
}

type handler func(resp http.ResponseWriter, req *http.Request) error

func (h handler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	runHandler(resp, req, h, handleError)
}

type apiHandler func(resp http.ResponseWriter, req *http.Request) error

func (h apiHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	runHandler(resp, req, h, handleAPIError)
}

func handleError(resp http.ResponseWriter, req *http.Request, status int, err error) {
	switch status {
	case http.StatusNotFound:
		executeTemplate(resp, "notfound"+templateExt(req), status, nil, nil)
	default:
		s := http.StatusText(status)
		if err == errUpdateTimeout {
			s = "Timeout getting package files from the version control system."
		} else if e, ok := err.(*gosrc.RemoteError); ok {
			s = "Error getting package files from " + e.Host + "."
		}
		resp.Header().Set("Content-Type", textMIMEType)
		resp.WriteHeader(http.StatusInternalServerError)
		io.WriteString(resp, s)
	}
}

func handleAPIError(resp http.ResponseWriter, req *http.Request, status int, err error) {
	var data struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	data.Error.Message = http.StatusText(status)
	resp.Header().Set("Content-Type", jsonMIMEType)
	resp.WriteHeader(status)
	json.NewEncoder(resp).Encode(&data)
}

type hostMux []struct {
	prefix string
	h      http.Handler
}

func (m hostMux) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var h http.Handler
	for _, ph := range m {
		if strings.HasPrefix(req.Host, ph.prefix) {
			h = ph.h
			break
		}
	}
	h.ServeHTTP(resp, req)
}

func defaultBase(path string) string {
	p, err := build.Default.Import(path, "", build.FindOnly)
	if err != nil {
		return "."
	}
	return p.Dir
}

var (
	db                 *database.Database
	statusImageHandler http.Handler
	srcFiles           = make(map[string]*zip.File)
)

var (
	robot             = flag.Float64("robot", 100, "Request counter threshold for robots")
	assetsDir         = flag.String("assets", filepath.Join(defaultBase("github.com/garyburd/gddo/gddo-server"), "assets"), "Base directory for templates and static files.")
	getTimeout        = flag.Duration("get_timeout", 8*time.Second, "Time to wait for package update from the VCS.")
	firstGetTimeout   = flag.Duration("first_get_timeout", 5*time.Second, "Time to wait for first fetch of package from the VCS.")
	maxAge            = flag.Duration("max_age", 24*time.Hour, "Update package documents older than this age.")
	httpAddr          = flag.String("http", ":8080", "Listen for HTTP connections on this address")
	srcZip            = flag.String("srcZip", "", "")
	sidebarEnabled    = flag.Bool("sidebar", false, "Enable package page sidebar.")
	gitHubCredentials = ""
	userAgent         = ""
)

func main() {
	flag.Parse()
	log.Printf("Starting server, os.Args=%s", strings.Join(os.Args, " "))

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
		{"importers_robot.html", "common.html", "layout.html"},
		{"imports.html", "common.html", "layout.html"},
		{"file.html", "common.html", "layout.html"},
		{"index.html", "common.html", "layout.html"},
		{"notfound.html", "common.html", "layout.html"},
		{"pkg.html", "common.html", "layout.html"},
		{"results.html", "common.html", "layout.html"},
		{"tools.html", "common.html", "layout.html"},
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
	}); err != nil {
		log.Fatal(err)
	}

	var err error
	db, err = database.New()
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	go runBackgroundTasks()

	cssFiles := []string{"third_party/bootstrap/css/bootstrap.min.css", "site.css"}
	if *sidebarEnabled {
		cssFiles = append(cssFiles, "sidebar.css")
	}

	staticServer := httputil.StaticServer{
		Dir:    *assetsDir,
		MaxAge: time.Hour,
		MIMETypes: map[string]string{
			".css": "text/css; charset=utf-8",
			".js":  "text/javascript; charset=utf-8",
		},
	}
	statusImageHandler = staticServer.FileHandler("status.png")

	apiMux := http.NewServeMux()
	apiMux.Handle("/favicon.ico", staticServer.FileHandler("favicon.ico"))
	apiMux.Handle("/google3d2f3cd4cc2bb44b.html", staticServer.FileHandler("google3d2f3cd4cc2bb44b.html"))
	apiMux.Handle("/humans.txt", staticServer.FileHandler("humans.txt"))
	apiMux.Handle("/robots.txt", staticServer.FileHandler("apiRobots.txt"))
	apiMux.Handle("/search", apiHandler(serveAPISearch))
	apiMux.Handle("/packages", apiHandler(serveAPIPackages))
	apiMux.Handle("/importers/", apiHandler(serveAPIImporters))
	apiMux.Handle("/imports/", apiHandler(serveAPIImports))
	apiMux.Handle("/", apiHandler(serveAPIHome))

	mux := http.NewServeMux()
	mux.Handle("/-/site.js", staticServer.FilesHandler(
		"third_party/jquery.timeago.js",
		"third_party/typeahead.min.js",
		"third_party/bootstrap/js/bootstrap.min.js",
		"site.js"))
	mux.Handle("/-/site.css", staticServer.FilesHandler(cssFiles...))
	mux.Handle("/-/about", handler(serveAbout))
	mux.Handle("/-/bot", handler(serveBot))
	mux.Handle("/-/go", handler(serveGoIndex))
	mux.Handle("/-/subrepo", handler(serveGoSubrepoIndex))
	mux.Handle("/-/index", handler(serveIndex))
	mux.Handle("/-/refresh", handler(serveRefresh))
	mux.Handle("/a/index", http.RedirectHandler("/-/index", 301))
	mux.Handle("/about", http.RedirectHandler("/-/about", 301))
	mux.Handle("/favicon.ico", staticServer.FileHandler("favicon.ico"))
	mux.Handle("/google3d2f3cd4cc2bb44b.html", staticServer.FileHandler("google3d2f3cd4cc2bb44b.html"))
	mux.Handle("/humans.txt", staticServer.FileHandler("humans.txt"))
	mux.Handle("/robots.txt", staticServer.FileHandler("robots.txt"))
	mux.Handle("/BingSiteAuth.xml", staticServer.FileHandler("BingSiteAuth.xml"))
	mux.Handle("/C", http.RedirectHandler("http://golang.org/doc/articles/c_go_cgo.html", 301))
	mux.Handle("/ajax.googleapis.com/", http.NotFoundHandler())
	mux.Handle("/", handler(serveHome))

	cacheBusters.Handler = mux

	if err := http.ListenAndServe(*httpAddr, hostMux{{"api.", apiMux}, {"", mux}}); err != nil {
		log.Fatal(err)
	}
}
