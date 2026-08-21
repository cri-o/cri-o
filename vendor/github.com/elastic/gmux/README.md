# gmux

gmux is a Go library for simultaneously serving net/http and gRPC requests on a single port.

gmux is derived from [cmux](https://github.com/soheilhy/cmux), enabling mTLS at the expense of
reduced flexibility. Whereas cmux operates at the transport layer and enables multiplexing
arbitrary application layer protocols, gmux operates at the application layer (on top of HTTP
with or without TLS), to enable better integration with net/http and crypto/tls.

## Example

```go
s := &http.Server{
	Addr:    ":8080",
	Handler: myHandler,
}

// Configure the server with gmux. The returned net.Listener will receive gRPC connections,
// while all other requests will be handled by s.Handler.
grpcListener, err := gmux.ConfigureServer(s, nil)
if err != nil {
	log.Fatal(err)
}

grpcServer := grpc.NewServer()
go grpcServer.Serve(grpcListener)
log.Fatal(s.ListenAndServeTLS("cert.pem", "key.pem"))
```

## License

This software is licensed under the [Apache License 2.0](./LICENSE).

