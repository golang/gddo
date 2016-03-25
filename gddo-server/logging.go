// Copyright 2016 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"google.golang.org/cloud/logging"
)

// NewLoggingHandler returns a handler that wraps h but logs each request
// using Google Cloud Logging service.
func NewLoggingHandler(h http.Handler, cli *logging.Client) http.Handler {
	return &loggingHandler{h, cli}
}

type loggingHandler struct {
	handler http.Handler
	cli     *logging.Client
}

func (h *loggingHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	t := time.Now().UTC()

	const sessionCookieName = "GODOC_ORG_SESSION_ID"
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		// Generates a random session id and sends it in response.
		r, err := randomString()
		if err != nil {
			// If error happens during generating the session id, proceed
			// without logging.
			log.Println("generating a random session id:", err)
			h.handler.ServeHTTP(resp, req)
			return
		}
		// This cookie is intentionally short-lived and contains no information
		// that might identify the user.  Its sole purpose is to tie query
		// terms and destination pages together to measure search quality.
		cookie = &http.Cookie{
			Name:    sessionCookieName,
			Value:   r,
			Expires: time.Now().Add(time.Hour),
		}
		http.SetCookie(resp, cookie)
	}

	w := &countingResponseWriter{
		ResponseWriter: resp,
		responseStatus: http.StatusOK,
	}
	h.handler.ServeHTTP(w, req)

	// We must not record the client's IP address, referrer URL, or any other
	// information that might compromise the user's privacy.
	entry := logging.Entry{
		Time: t,
		Payload: map[string]interface{}{
			sessionCookieName: cookie.Value,
			"latency":         time.Since(t),
			"path":            req.URL.RequestURI(),
			"method":          req.Method,
			"responseBytes":   w.responseBytes,
			"status":          w.responseStatus,
		},
	}
	// Log queues the entry to its internal buffer, or discarding the entry
	// if the buffer was full.
	h.cli.Log(entry)
}

func randomString() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

// A countingResponseWriter is a wrapper around an http.ResponseWriter that
// records the number of bytes written and the status of the response.
type countingResponseWriter struct {
	http.ResponseWriter

	responseBytes  int64
	responseStatus int
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	written, err := w.ResponseWriter.Write(p)
	w.responseBytes += int64(written)
	return written, err
}

func (w *countingResponseWriter) WriteHeader(status int) {
	w.responseStatus = status
	w.ResponseWriter.WriteHeader(status)
}
