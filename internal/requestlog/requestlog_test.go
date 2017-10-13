// Copyright 2017 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package requestlog

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	const requestMsg = "Hello, World!"
	const responseMsg = "I see you."
	const userAgent = "Request Log Test UA"
	const referer = "http://www.example.com/"

	r, err := http.NewRequest("POST", "http://localhost/foo", strings.NewReader(requestMsg))
	if err != nil {
		t.Fatal("NewRequest:", err)
	}
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("Referer", referer)
	requestHdrSize := len(fmt.Sprintf("User-Agent: %s\r\nReferer: %s\r\nContent-Length: %v\r\n", userAgent, referer, len(requestMsg)))
	responseHdrSize := len(fmt.Sprintf("Content-Length: %v\r\n", len(responseMsg)))
	ent, err := roundTrip(r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(responseMsg)))
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, responseMsg)
	}))
	if err != nil {
		t.Fatal("Could not get entry:", err)
	}
	if want := "POST"; ent.RequestMethod != want {
		t.Errorf("RequestMethod = %q; want %q", ent.RequestMethod, want)
	}
	if want := "/foo"; ent.RequestURL != want {
		t.Errorf("RequestURL = %q; want %q", ent.RequestURL, want)
	}
	if ent.RequestHeaderSize < int64(requestHdrSize) {
		t.Errorf("RequestHeaderSize = %d; want >=%d", ent.RequestHeaderSize, requestHdrSize)
	}
	if ent.RequestBodySize != int64(len(requestMsg)) {
		t.Errorf("RequestBodySize = %d; want %d", ent.RequestBodySize, len(requestMsg))
	}
	if ent.UserAgent != userAgent {
		t.Errorf("UserAgent = %q; want %q", ent.UserAgent, userAgent)
	}
	if ent.Referer != referer {
		t.Errorf("Referer = %q; want %q", ent.Referer, referer)
	}
	if want := "HTTP/1.1"; ent.Proto != want {
		t.Errorf("Proto = %q; want %q", ent.Proto, want)
	}
	if ent.Status != http.StatusOK {
		t.Errorf("Status = %d; want %d", ent.Status, http.StatusOK)
	}
	if ent.ResponseHeaderSize < int64(responseHdrSize) {
		t.Errorf("ResponseHeaderSize = %d; want >=%d", ent.ResponseHeaderSize, responseHdrSize)
	}
	if ent.ResponseBodySize != int64(len(responseMsg)) {
		t.Errorf("ResponseBodySize = %d; want %d", ent.ResponseBodySize, len(responseMsg))
	}
}

func roundTrip(r *http.Request, h http.Handler) (*Entry, error) {
	capture := new(captureLogger)
	hh := NewHandler(capture, h)
	s := httptest.NewServer(hh)
	defer s.Close()
	r.URL.Host = s.URL[len("http://"):]
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return &capture.ent, nil
}

type captureLogger struct {
	ent Entry
}

func (cl *captureLogger) Log(ent *Entry) {
	cl.ent = *ent
}
