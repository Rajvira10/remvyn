# Builds the Linux x86_64 binary. Ubuntu 22.04's older glibc keeps the binary
# runnable on newer distros too (Arch/Omarchy, Fedora, Debian 12+, Ubuntu 22.04+).
#   docker build --platform linux/amd64 -t remvyn-linux-build packaging/linux
#   packaging/linux/build.sh
FROM --platform=linux/amd64 ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential pkg-config ca-certificates curl git xz-utils \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://go.dev/dl/go1.26.10.linux-amd64.tar.gz | tar -C /usr/local -xz
RUN curl -fsSL https://nodejs.org/dist/v22.20.0/node-v22.20.0-linux-x64.tar.xz | tar -C /usr/local --strip-components=1 -xJ
ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH
RUN go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
