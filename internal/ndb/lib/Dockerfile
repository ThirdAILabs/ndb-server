# This can be used to build thirdai linux libraries that can be used in the docker images. 
# Example: 
# 1. Run `docker build -t linux-libs -f Dockerfile ../` if not running on x86_64 use --platform linux/amd64
# 2. Run `docker run --rm -v /home/<username>/linux_amd64:/app/output linux-libs cp -r /app/lib /app/output`

FROM golang:1.24.4

WORKDIR /app

RUN apt-get update -y && apt-get install -y gcc-11 g++-11 cmake zlib1g-dev python3-dev

COPY ./Universe /app/Universe

RUN rm -rf /app/Universe/build

RUN export CXX=g++-11 && export CC=gcc-11 && /app/Universe/bin/build.py -t thirdai

RUN mkdir -p /app/lib

RUN cp /app/Universe/build/libthirdai.a /app/lib
RUN cp /app/Universe/build/deps/rocksdb/librocksdb.a /app/lib
RUN cp /app/Universe/build/deps/utf8proc/libutf8proc.a /app/lib
RUN cp /app/Universe/build/deps/spdlog/libspdlog.a /app/lib
