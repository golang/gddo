// Copyright 2013 The Go Authors. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file or at
// https://developers.google.com/open-source/licenses/bsd.

package main

import (
	"reflect"
	"testing"

	"github.com/golang/gddo/database"
	"github.com/golang/gddo/doc"
)

var robotTests = []string{
	"Mozilla/5.0 (compatible; TweetedTimes Bot/1.0; +http://tweetedtimes.com)",
	"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
	"Mozilla/5.0 (compatible; MJ12bot/v1.4.3; http://www.majestic12.co.uk/bot.php?+)",
	"Go 1.1 package http",
	"Java/1.7.0_25	0.003	0.003",
	"Python-urllib/2.6",
	"Mozilla/5.0 (compatible; archive.org_bot +http://www.archive.org/details/archive.org_bot)",
	"Mozilla/5.0 (compatible; Ezooms/1.0; ezooms.bot@gmail.com)",
	"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
}

func TestRobotPat(t *testing.T) {
	// TODO(light): isRobot checks for more than just the User-Agent.
	// Extract out the database interaction to an interface to test the
	// full analysis.

	for _, tt := range robotTests {
		if !robotPat.MatchString(tt) {
			t.Errorf("%s not a robot", tt)
		}
	}
}

func TestRemoveInternalPkgs(t *testing.T) {
	tests := []struct {
		name string
		pdoc *doc.Package
		pkgs []database.Package
		want []database.Package
	}{
		{
			name: "no children",
			pdoc: &doc.Package{
				ImportPath: "github.com/user/repo",
				Name:       "repo",
			},
			pkgs: []database.Package{},
			want: []database.Package{},
		},
		{
			name: "indirect internal children",
			pdoc: &doc.Package{
				ImportPath: "github.com/user/repo",
				Name:       "repo",
			},
			pkgs: []database.Package{
				{Name: "agent", Path: "github.com/user/repo/cmd/internal/agent"},
				{Name: "agent", Path: "github.com/user/repo/cmd/internal/reporter"},
				{Name: "tool", Path: "github.com/user/repo/cmd/tool"},
			},
			want: []database.Package{
				{Name: "tool", Path: "github.com/user/repo/cmd/tool"},
			},
		},
		{
			name: "direct internal children",
			pdoc: &doc.Package{
				ImportPath: "github.com/user/repo",
				Name:       "repo",
			},
			pkgs: []database.Package{
				{Name: "agent", Path: "github.com/user/repo/internal/agent"},
				{Name: "agent", Path: "github.com/user/repo/internal/reporter"},
				{Name: "tool", Path: "github.com/user/repo/cmd/tool"},
			},
			want: []database.Package{
				{Name: "agent", Path: "github.com/user/repo/internal/agent"},
				{Name: "agent", Path: "github.com/user/repo/internal/reporter"},
				{Name: "tool", Path: "github.com/user/repo/cmd/tool"},
			},
		},
		{
			name: "internal package",
			pdoc: &doc.Package{
				ImportPath: "github.com/user/repo/internal",
				Name:       "internal",
			},
			pkgs: []database.Package{
				{Name: "agent", Path: "github.com/user/repo/internal/agent"},
				{Name: "tool", Path: "github.com/user/repo/internal/tool"},
			},
			want: []database.Package{
				{Name: "agent", Path: "github.com/user/repo/internal/agent"},
				{Name: "tool", Path: "github.com/user/repo/internal/tool"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := removeInternal(tt.pdoc, tt.pkgs), tt.want
			if !reflect.DeepEqual(got, want) {
				t.Errorf("removeInternal() = %v, want %v", got, tt.want)
			}
		})
	}
}
