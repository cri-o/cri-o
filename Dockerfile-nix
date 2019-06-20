# vim: set syntax=dockerfile:
FROM nixos/nix
RUN apk add git
COPY . /cri-o
ARG COMMIT
WORKDIR cri-o/nix
RUN nix-build --argstr revision ${COMMIT}
WORKDIR /
RUN rm -rf cri-o
