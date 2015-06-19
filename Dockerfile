FROM golang:1.3-onbuild

EXPOSE 9102
EXPOSE 9125/udp

VOLUME ["/mapping.conf"]

ENTRYPOINT ["/go/bin/app"]
