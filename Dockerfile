FROM google/golang

# Install redis, nginx, daemontools, etc.
RUN echo deb http://http.debian.net/debian wheezy-backports main > /etc/apt/sources.list.d/backports.list
RUN apt-get update
RUN apt-get install -y --no-install-recommends -t wheezy-backports redis-server
RUN apt-get install -y --no-install-recommends graphviz nginx-full daemontools unzip

# Configure redis.
ADD deploy/redis.conf /etc/redis/redis.conf

# Configure nginx.
RUN echo "daemon off;" >> /etc/nginx/nginx.conf
RUN rm /etc/nginx/sites-enabled/default
ADD deploy/gddo.conf /etc/nginx/sites-enabled/gddo.conf

# Configure daemontools services.
ADD deploy/services /services

# Manually fetch and install gddo-server dependencies (faster than "go get").
# redigo
RUN curl -L https://github.com/garyburd/redigo/archive/779af66db5668074a96f522d9025cb0a5ef50d89.zip > redigo.zip
RUN unzip redigo.zip
RUN mkdir -p /gopath/src/github.com/garyburd
RUN mv redigo-* /gopath/src/github.com/garyburd/redigo
# snappy-go
RUN curl https://snappy-go.googlecode.com/archive/12e4b4183793ac4b061921e7980845e750679fd0.tar.gz | tar xz
RUN mkdir -p /gopath/src/code.google.com/p
RUN mv snappy-go-* /gopath/src/code.google.com/p/snappy-go

# Build the local gddo files.
ADD . /gopath/src/github.com/golang/gddo
RUN go install github.com/golang/gddo/gddo-server

# Exposed ports and volumes.
# /ssl should contain SSL certs.
# /data should contain the Redis database, "dump.rdb".
EXPOSE 80 443
VOLUME ["/ssl", "/data"]

# How to start it all.
CMD svscan /services
