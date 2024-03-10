FROM golang:1.21-alpine3.19 AS build
WORKDIR $GOPATH/github.com/frantjc/momo
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED 0
RUN go build -o /momo ./cmd/momo

FROM amazoncorretto:21-alpine3.19
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/master/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool
COPY assets/ /usr/local/bin
RUN chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo", "srv"]
COPY --from=build momo /usr/local/bin
