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
	"net/http"
	"testing"
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

func TestRobots(t *testing.T) {
	for _, tt := range robotTests {
		req := http.Request{Header: http.Header{"User-Agent": {tt}}}
		if !isRobot(&req) {
			t.Errorf("%s not a robot", tt)
		}
	}
}
