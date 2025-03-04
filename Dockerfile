FROM golang:1.23-alpine AS build
WORKDIR $GOPATH/github.com/frantjc/momo
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
COPY android/ android/
COPY api/ api/
COPY apktool/ apktool/
COPY cmd/ cmd/
COPY command/ command/
COPY internal/ internal/
COPY ios/ ios/
COPY keytool/ keytool/
RUN CGO_ENABLED=0 go build -o /momo ./cmd/momo

FROM node:20.11.1-alpine AS remix
WORKDIR /src/github.com/frantjc/momo
COPY package.json yarn.lock ./
RUN yarn
COPY app/ app/
COPY public/ public/
COPY *.js *.ts tsconfig.json ./
RUN yarn build

FROM amazoncorretto:21-alpine
ENV NODE_VERSION 20.11.1
RUN apk add --no-cache \
        libstdc++ \
    && apk add --no-cache --virtual .build-deps \
        curl \
    && set -eu; \
        curl -fsSLO --compressed "https://unofficial-builds.nodejs.org/download/release/v$NODE_VERSION/node-v$NODE_VERSION-linux-x64-musl.tar.xz"; \
        echo "5da733c21c3b51193a4fe9fc5be6cfa9a694d13b8d766eb02dbe4b8996547050 node-v$NODE_VERSION-linux-x64-musl.tar.xz" | sha256sum -c - \
            && tar -xJf "node-v$NODE_VERSION-linux-x64-musl.tar.xz" -C /usr/local --strip-components=1 --no-same-owner; \
    rm -f "node-v$NODE_VERSION-linux-x64-musl.tar.xz" \
        && find /usr/local/include/node/openssl/archs -mindepth 1 -maxdepth 1 ! -name "linux-x86_64" -exec rm -rf {} \; \
        && apk del .build-deps
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/v2.9.3/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool \
  && chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo"]
COPY server.js package.json /app/
COPY --from=remix /src/github.com/frantjc/momo/build /app/build/
COPY --from=remix /src/github.com/frantjc/momo/node_modules /app/node_modules/
COPY --from=remix /src/github.com/frantjc/momo/public /app/public/
COPY --from=build momo /usr/local/bin
