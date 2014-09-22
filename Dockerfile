FROM google/golang

RUN echo deb http://http.debian.net/debian wheezy-backports main > /etc/apt/sources.list.d/backports.list
RUN apt-get update
RUN apt-get install -y --no-install-recommends -t wheezy-backports redis-server
RUN apt-get install -y --no-install-recommends git graphviz nginx-full daemontools

ADD deploy/redis.conf /etc/redis/redis.conf

RUN echo "daemon off;" >> /etc/nginx/nginx.conf
RUN rm /etc/nginx/sites-enabled/default
ADD deploy/gddo.conf /etc/nginx/sites-enabled/gddo.conf

ADD deploy/services /services

ADD . /gopath/src/github.com/golang/gddo

RUN go get github.com/golang/gddo/gddo-server

EXPOSE 80 443
VOLUME ["/ssl", "/data"]

CMD svscan /services
