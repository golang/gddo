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

// Command gddo-admin is the GoDoc.org command line administration tool.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type command struct {
	name  string
	run   func(c *command)
	flag  flag.FlagSet
	usage string
}

func (c *command) printUsage() {
	fmt.Fprintf(os.Stderr, "%s %s\n", os.Args[0], c.usage)
	c.flag.PrintDefaults()
}

var commands = []*command{
	blockCommand,
	reindexCommand,
	pruneCommand,
	deleteCommand,
	popularCommand,
	dangleCommand,
	crawlCommand,
	statsCommand,
}

func printUsage() {
	var n []string
	for _, c := range commands {
		n = append(n, c.name)
	}
	fmt.Fprintf(os.Stderr, "%s %s\n", os.Args[0], strings.Join(n, "|"))
	flag.PrintDefaults()
	for _, c := range commands {
		c.printUsage()
	}
}

func main() {
	flag.Usage = printUsage
	flag.Parse()
	args := flag.Args()
	if len(args) >= 1 {
		for _, c := range commands {
			if args[0] == c.name {
				c.flag.Usage = func() {
					c.printUsage()
					os.Exit(2)
				}
				c.flag.Parse(args[1:])
				c.run(c)
				return
			}
		}
	}
	printUsage()
	os.Exit(2)
}
