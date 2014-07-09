#!/bin/sh
go get code.google.com/p/go.tools/godoc
present=`go list -f '{{.Dir}}' code.google.com/p/go.tools/cmd/present`
godoc=`go list -f '{{.Dir}}' code.google.com/p/go.tools/godoc`
mkdir -p present

(cat $godoc/static/jquery.js $godoc/static/playground.js $godoc/static/play.js && echo "initPlayground(new HTTPTransport());") > present/play.js

cd ./present
for i in templates static
do
    ln -is $present/$i
done
