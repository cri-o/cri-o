FROM golang:1.13.1-alpine3.10
RUN apk add --no-cache git
WORKDIR /home
COPY ./ .
RUN CGO_ENABLED=0 GOOS=linux go build -o go-mod-outdated .

FROM scratch
WORKDIR /home/
COPY --from=0 /home/go-mod-outdated .
ENTRYPOINT ["./go-mod-outdated"]
