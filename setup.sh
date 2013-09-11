#!/bin/sh
go get code.google.com/p/go.talks/present
present=`go list -f '{{.Dir}}' code.google.com/p/go.talks/present`
mkdir -p present

(cat $present/js/jquery.js $present/js/jquery-ui.js $present/js/playground.js $present/js/play.js && echo "initPlayground(new HTTPTransport());") > present/play.js

cd present
for i in templates static
do
    ln -is $present/$i
done
