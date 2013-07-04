This project is the source for http://godoc.org/

The code in this project is designed to be used by godoc.org. Send mail to
info@godoc.org if you want to discuss other uses of the code.

Feedback
--------

Send ideas and questions to info@godoc.org. Request features and report bugs
using the [Github Issue
Tracker](https://github.com/garyburd/gopkgdoc/issues/new). 


Contributing
------------

Contributions are welcome. 

Before writing code, send mail to info@godoc.org to discuss what you plan to
do. This gives the project manager a chance to validate the design, avoid
duplication of effort and ensure that the changes fit the goals of the project.
Do not start the discussion with a pull request. 

Development Environment Setup
-----------------------------

- Install and run [Redis 2.6.x](http://redis.io/download). The redis.conf file included in the Redis distribution is suitable for development.
- Install Go from source and update to tip.
- Install and run the server:

        go get github.com/garyburd/gopkgdoc/gddo-server
        gddo-server

License
-------

[Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0.html).
