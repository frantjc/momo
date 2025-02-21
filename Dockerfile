FROM golang:1.23-alpine AS build
WORKDIR $GOPATH/github.com/frantjc/momo
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
COPY android/ android/
COPY api/ api/
COPY apktool/ apktool/
COPY cmd/ cmd/
COPY internal/ internal/
COPY ios/ ios/
COPY keytool/ keytool/
RUN CGO_ENABLED=0 go build -o /momo ./cmd/momo

FROM amazoncorretto:21-alpine
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/v2.9.3/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool \
  && chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo"]
COPY --from=build momo /usr/local/bin
