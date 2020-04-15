FROM golang:1.14.2-alpine3.11
RUN apk add --no-cache git
WORKDIR /home
COPY ./ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o go-mod-outdated .

FROM scratch
WORKDIR /home/
COPY --from=0 /home/go-mod-outdated .
ENTRYPOINT ["./go-mod-outdated"]
