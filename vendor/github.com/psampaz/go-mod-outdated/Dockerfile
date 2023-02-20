FROM golang:1.20.1-alpine3.17
RUN apk update
RUN apk add --no-cache git

FROM scratch
WORKDIR /home/
COPY go-mod-outdated /usr/bin/
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["go-mod-outdated"]
