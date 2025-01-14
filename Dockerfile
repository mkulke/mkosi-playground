# syntax=docker/dockerfile:1.3-labs
FROM ubuntu:24.04 AS builder

ARG MKOSI_VERSION="v22"

RUN apt-get update
RUN apt-get install -y \
  alien \
  btrfs-progs \
  bubblewrap \
  build-essential \
  dnf \
  dosfstools \
  git \
  kmod \
  mtools \
  pipx \
  qemu-utils \
  sbsigntool \
  squashfs-tools \
  systemd-ukify \
  uidmap \
  zstd

RUN mkdir -p /build
WORKDIR /build
RUN pipx install "git+https://github.com/systemd/mkosi.git@${MKOSI_VERSION}"
RUN pipx inject mkosi cryptography
RUN cp /root/.local/bin/mkosi /usr/local/bin/mkosi
RUN mkosi --version

RUN mkdir /image
WORKDIR /image
COPY initrd /image/initrd
COPY mkosi.conf /image/mkosi.conf
COPY ssh /image/ssh

RUN mkosi genkey
RUN --security=insecure mkosi -C ./initrd build
RUN --security=insecure mkosi build

FROM scratch
COPY --from=builder /image/image.raw /image.raw
