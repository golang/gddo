// Copyright 2017 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package requestlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFluentdLog(t *testing.T) {
	const (
		startTime      = 1507914000
		startTimeNanos = 512

		latencySec   = 5
		latencyNanos = 123456789

		endTime      = startTime + latencySec
		endTimeNanos = startTimeNanos + latencyNanos
	)
	buf := new(bytes.Buffer)
	var logErr error
	l := NewFluentdLogger(buf, "mytag", func(e error) { logErr = e })
	want := &Entry{
		ReceivedTime:       time.Unix(startTime, startTimeNanos),
		RequestMethod:      "POST",
		RequestURL:         "/foo/bar",
		RequestHeaderSize:  456,
		RequestBodySize:    123000,
		UserAgent:          "Chrome proxied through Firefox and Edge",
		Referer:            "http://www.example.com/",
		Proto:              "HTTP/1.1",
		RemoteIP:           "12.34.56.78",
		ServerIP:           "127.0.0.1",
		Status:             404,
		ResponseHeaderSize: 555,
		ResponseBodySize:   789000,
		Latency:            latencySec*time.Second + latencyNanos*time.Nanosecond,
	}
	ent := *want // copy in case Log accidentally mutates
	l.Log(&ent)
	if logErr != nil {
		t.Error("Logger called error callback:", logErr)
	}

	var got []json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal("Unmarshal:", err)
	}

	if len(got) == 0 {
		t.Fatal("Message is empty; want 3 elements")
	}
	var tag string
	if err := json.Unmarshal(got[0], &tag); err != nil {
		t.Error("Unmarshal tag:", err)
	} else if want := "mytag"; tag != want {
		t.Errorf("tag = %q; want %q", tag, want)
	}

	if len(got) < 2 {
		t.Fatal("Message only has 1 element; want 3 elements")
	}
	var timestamp int64
	if err := json.Unmarshal(got[1], &timestamp); err != nil {
		t.Error("Unmarshal timestamp:", err)
	} else if want := int64(endTime); timestamp != want {
		t.Errorf("timestamp = %d; want %d", timestamp, want)
	}

	if len(got) < 3 {
		t.Fatal("Message only has 2 elements; want 3 elements")
	}
	var r map[string]interface{}
	if err := json.Unmarshal(got[2], &r); err != nil {
		t.Error("Unmarshal record:", err)
	} else {
		rr, _ := r["httpRequest"].(map[string]interface{})
		if rr == nil {
			t.Error("httpRequest does not exist in record or is not a JSON object")
		}
		if got, want := jsonString(rr, "requestMethod"), ent.RequestMethod; got != want {
			t.Errorf("httpRequest.requestMethod = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "requestUrl"), ent.RequestURL; got != want {
			t.Errorf("httpRequest.requestUrl = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "requestSize"), "123456"; got != want {
			t.Errorf("httpRequest.requestSize = %q; want %q", got, want)
		}
		if got, want := jsonNumber(rr, "status"), float64(ent.Status); got != want {
			t.Errorf("httpRequest.status = %d; want %d", int64(got), int64(want))
		}
		if got, want := jsonString(rr, "responseSize"), "789555"; got != want {
			t.Errorf("httpRequest.responseSize = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "userAgent"), ent.UserAgent; got != want {
			t.Errorf("httpRequest.userAgent = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "remoteIp"), ent.RemoteIP; got != want {
			t.Errorf("httpRequest.remoteIp = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "referer"), ent.Referer; got != want {
			t.Errorf("httpRequest.referer = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "latency"), "5.123456789"; parseLatency(got) != want {
			t.Errorf("httpRequest.latency = %q; want %q", got, want+"s")
		}
		ts, _ := r["timestamp"].(map[string]interface{})
		if ts == nil {
			t.Error("timestamp does not exist in record or is not a JSON object")
		}
		if got, want := jsonNumber(ts, "seconds"), float64(endTime); got != want {
			t.Errorf("timestamp.seconds = %g; want %g", got, want)
		}
		if got, want := jsonNumber(ts, "nanos"), float64(endTimeNanos); got != want {
			t.Errorf("timestamp.nanos = %g; want %g", got, want)
		}
	}
	if len(got) > 3 {
		t.Errorf("Message has %d elements; want 3 elements", len(got))
	}
}

func parseLatency(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "s") {
		return ""
	}
	s = strings.TrimSpace(s[:len(s)-1])
	for _, c := range s {
		if !(c >= '0' && c <= '9') && c != '.' {
			return ""
		}
	}
	return s
}

func jsonString(obj map[string]interface{}, k string) string {
	v, _ := obj[k].(string)
	return v
}

func jsonNumber(obj map[string]interface{}, k string) float64 {
	v, _ := obj[k].(float64)
	return v
}

func BenchmarkFluentdLog(b *testing.B) {
	ent := &Entry{
		ReceivedTime:       time.Date(2017, time.October, 13, 17, 0, 0, 512, time.UTC),
		RequestMethod:      "POST",
		RequestURL:         "/foo/bar",
		RequestHeaderSize:  456,
		RequestBodySize:    123000,
		UserAgent:          "Chrome proxied through Firefox and Edge",
		Referer:            "http://www.example.com/",
		Proto:              "HTTP/1.1",
		RemoteIP:           "12.34.56.78",
		ServerIP:           "127.0.0.1",
		Status:             404,
		ResponseHeaderSize: 555,
		ResponseBodySize:   789000,
		Latency:            5 * time.Second,
	}
	var buf bytes.Buffer
	l := NewFluentdLogger(&buf, "mytag", func(error) {})
	l.Log(ent)
	b.ReportAllocs()
	b.SetBytes(int64(buf.Len()))
	buf.Reset()
	b.ResetTimer()

	l = NewFluentdLogger(ioutil.Discard, "mytag", func(error) {})
	for i := 0; i < b.N; i++ {
		l.Log(ent)
	}
}

func BenchmarkE2E(b *testing.B) {
	run := func(b *testing.B, handler http.Handler) {
		s := httptest.NewServer(handler)
		defer s.Close()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resp, err := s.Client().Get(s.URL)
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	b.Run("Baseline", func(b *testing.B) {
		run(b, http.HandlerFunc(benchHandler))
	})
	b.Run("WithLog", func(b *testing.B) {
		l := NewFluentdLogger(ioutil.Discard, "mytag", func(error) {})
		run(b, NewHandler(l, http.HandlerFunc(benchHandler)))
	})
}

func benchHandler(w http.ResponseWriter, r *http.Request) {
	const msg = "Hello, World!"
	w.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	io.WriteString(w, msg)
}
