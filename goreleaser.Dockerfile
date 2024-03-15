FROM node:20.11.1-alpine3.19 as remix
WORKDIR /src/github.com/frantjc/momo
COPY package.json yarn.lock ./
RUN yarn
COPY app/ app/
COPY public/ public/
COPY *.js *.ts tsconfig.json ./
RUN yarn build

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

FROM base
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/master/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool
COPY assets/ /usr/local/bin
RUN chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo", "srv", "/usr/local/bin/node", "/app/server.js"]
COPY server.js package.json /app/
COPY --from=remix /src/github.com/frantjc/momo/build /app/build/
COPY --from=remix /src/github.com/frantjc/momo/node_modules /app/node_modules/
COPY --from=remix /src/github.com/frantjc/momo/public /app/public/
COPY --from=go momo /usr/local/bin
