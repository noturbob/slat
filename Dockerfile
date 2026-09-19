# The image published to ghcr.io/noturbob/slat by GoReleaser, for trying
# slat without installing it:  docker run -it ghcr.io/noturbob/slat
# GoReleaser supplies the prebuilt binary; nothing is compiled here.
FROM alpine:3
ARG TARGETPLATFORM
RUN apk add --no-cache bash htop ncurses-terminfo-base
ENV SHELL=/bin/bash TERM=xterm-256color
WORKDIR /root
COPY $TARGETPLATFORM/slat /usr/bin/slat
ENTRYPOINT ["/usr/bin/slat"]
