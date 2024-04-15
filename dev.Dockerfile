FROM golang:1.22-alpine3.19 AS go
WORKDIR $GOPATH/github.com/frantjc/momo
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
COPY android/ android/
COPY apktool/ apktool/
COPY cmd/ cmd/
COPY command/ command/
COPY internal/ internal/
COPY ios/ ios/
ENV CGO_ENABLED 0
RUN go build -o /momo ./cmd/momo

FROM amazoncorretto:21-alpine3.19 AS base
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

ENV YARN_VERSION 1.22.19
RUN apk add --no-cache --virtual .build-deps-yarn curl gnupg tar \
  && export GNUPGHOME="$(mktemp -d)" \
  && for key in \
    6A010C5166006599AA17F08146C2130DFD2497F5 \
  ; do \
    gpg --batch --keyserver hkps://keys.openpgp.org --recv-keys "$key" || \
    gpg --batch --keyserver keyserver.ubuntu.com --recv-keys "$key" ; \
  done \
  && curl -fsSLO --compressed "https://yarnpkg.com/downloads/$YARN_VERSION/yarn-v$YARN_VERSION.tar.gz" \
  && curl -fsSLO --compressed "https://yarnpkg.com/downloads/$YARN_VERSION/yarn-v$YARN_VERSION.tar.gz.asc" \
  && gpg --batch --verify yarn-v$YARN_VERSION.tar.gz.asc yarn-v$YARN_VERSION.tar.gz \
  && gpgconf --kill all \
  && rm -rf "$GNUPGHOME" \
  && mkdir -p /opt \
  && tar -xzf yarn-v$YARN_VERSION.tar.gz -C /opt/ \
  && ln -s /opt/yarn-v$YARN_VERSION/bin/yarn /usr/local/bin/yarn \
  && ln -s /opt/yarn-v$YARN_VERSION/bin/yarnpkg /usr/local/bin/yarnpkg \
  && rm yarn-v$YARN_VERSION.tar.gz.asc yarn-v$YARN_VERSION.tar.gz \
  && apk del .build-deps-yarn

FROM base
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/master/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool
COPY assets/ /usr/local/bin
RUN chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo", "srv"]
COPY --from=go momo /usr/local/bin
