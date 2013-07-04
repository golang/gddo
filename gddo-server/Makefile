BOOTSTRAP=$(HOME)/src/bootstrap
TIMEAGO=$(HOME)/src/jquery-timeago

default: assets/static/site.js assets/static/css/bootstrap.css

assets/static/css/bootstrap.css: less/after.less less/before.less less/bootstrap.less less/variables.less
	lessc -I$(BOOTSTRAP)/less -Iless less/bootstrap.less > assets/static/css/bootstrap.css

assets/static/site.js: \
	$(BOOTSTRAP)/bootstrap/js/bootstrap.min.js \
    	$(TIMEAGO)/jquery.timeago.js \
	js/site.js
	cat $^ > $@
