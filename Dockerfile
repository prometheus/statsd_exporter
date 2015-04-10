FROM golang:1.3-onbuild

EXPOSE 9102
EXPOSE 9125

VOLUME ["/mapping.conf"]

ENTRYPOINT ["/go/bin/app"]
