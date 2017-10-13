// Copyright 2017 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package requestlog

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"time"
)

// A FluentdLogger writes log entries in the Fluentd forward JSON
// format.  The record's fields are suitable for consumption by
// Stackdriver Logging.
type FluentdLogger struct {
	tag   string
	onErr func(error)

	mu  sync.Mutex
	w   io.Writer
	buf bytes.Buffer
	enc *json.Encoder
}

// NewFluentdLogger returns a new logger that writes to w.
func NewFluentdLogger(w io.Writer, tag string, onErr func(error)) *FluentdLogger {
	l := &FluentdLogger{
		tag:   tag,
		w:     w,
		onErr: onErr,
	}
	l.enc = json.NewEncoder(&l.buf)
	return l
}

// Log writes a record to its writer.  Multiple concurrent calls will
// produce sequential writes to its writer.
func (l *FluentdLogger) Log(ent *Entry) {
	if err := l.log(ent); err != nil {
		l.onErr(err)
	}
}

func (l *FluentdLogger) log(ent *Entry) error {
	defer l.mu.Unlock()
	l.mu.Lock()

	l.buf.Reset()
	l.buf.WriteByte('[')
	if err := l.enc.Encode(l.tag); err != nil {
		return err
	}
	l.buf.WriteByte(',')
	t := ent.ReceivedTime.Add(ent.Latency)
	if err := l.enc.Encode(t.Unix()); err != nil {
		return err
	}
	l.buf.WriteByte(',')

	var r struct {
		HTTPRequest struct {
			RequestMethod string `json:"requestMethod"`
			RequestURL    string `json:"requestUrl"`
			RequestSize   int64  `json:"requestSize,string"`
			Status        int    `json:"status"`
			ResponseSize  int64  `json:"responseSize,string"`
			UserAgent     string `json:"userAgent"`
			RemoteIP      string `json:"remoteIp"`
			Referer       string `json:"referer"`
			Latency       string `json:"latency"`
		} `json:"httpRequest"`
		Timestamp struct {
			Seconds int64 `json:"seconds"`
			Nanos   int   `json:"nanos"`
		} `json:"timestamp"`
	}
	r.HTTPRequest.RequestMethod = ent.RequestMethod
	r.HTTPRequest.RequestURL = ent.RequestURL
	// TODO(light): determine whether this is the formula LogEntry expects.
	r.HTTPRequest.RequestSize = ent.RequestHeaderSize + ent.RequestBodySize
	r.HTTPRequest.Status = ent.Status
	// TODO(light): determine whether this is the formula LogEntry expects.
	r.HTTPRequest.ResponseSize = ent.ResponseHeaderSize + ent.ResponseBodySize
	r.HTTPRequest.UserAgent = ent.UserAgent
	r.HTTPRequest.RemoteIP = ent.RemoteIP
	r.HTTPRequest.Referer = ent.Referer
	r.HTTPRequest.Latency = string(appendLatency(nil, ent.Latency))
	r.Timestamp.Seconds = t.Unix()
	r.Timestamp.Nanos = t.Nanosecond()
	if err := l.enc.Encode(r); err != nil {
		return err
	}
	l.buf.WriteByte(']')
	_, err := l.w.Write(l.buf.Bytes())
	return err
}

func appendLatency(b []byte, d time.Duration) []byte {
	// Parses format understood by google-fluentd (which is looser than the documented LogEntry format).
	// See the comment at https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/e2f60cdd1d97e79ffe4e91bdbf6bd84837f27fa5/lib/fluent/plugin/out_google_cloud.rb#L1539
	b = strconv.AppendFloat(b, d.Seconds(), 'f', 9, 64)
	b = append(b, 's')
	return b
}
