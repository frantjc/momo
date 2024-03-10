FROM amazoncorretto:21-alpine3.19
ADD https://bitbucket.org/iBotPeaches/apktool/downloads/apktool_2.9.3.jar /usr/local/bin/apktool.jar
ADD https://raw.githubusercontent.com/iBotPeaches/Apktool/master/scripts/linux/apktool /usr/local/bin/
RUN sed -i 's|#!/bin/bash|#!/bin/sh|g' /usr/local/bin/apktool
RUN chmod +x /usr/local/bin/*
ENTRYPOINT ["/usr/local/bin/momo"]
COPY momo /usr/local/bin
