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
	"fmt"
	"log"
	"os"

	"github.com/garyburd/gddo/database"
	"github.com/garyburd/redigo/redis"
)

var crawlCommand = &command{
	name:  "crawl",
	run:   crawl,
	usage: "crawl",
}

func crawl(c *command) {
	if len(c.flag.Args()) != 0 {
		c.printUsage()
		os.Exit(1)
	}
	db, err := database.New()
	if err != nil {
		log.Fatal(err)
	}
	conn := db.Pool.Get()
	defer conn.Close()

	paths, err := redis.Strings(conn.Do("SMEMBERS", "newCrawl"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("NEW")
	for _, path := range paths {
		fmt.Println(path)
	}

	paths, err = redis.Strings(conn.Do("SMEMBERS", "badCrawl"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("BAD")
	for _, path := range paths {
		fmt.Println(path)
	}
}
