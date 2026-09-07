// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package gmux

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const (
	http2FrameHeaderLength = 9

	grpcContentType = "application/grpc"

	// backendInitialSettingsTimeout bounds the one-time startup probe that
	// introspects the backend server's initial HTTP/2 SETTINGS frame.
	backendInitialSettingsTimeout = 5 * time.Second
)

// mux supports multiplexing plain-old HTTP/2 and gRPC traffic
// on a single listener.
type mux struct {
	http2Server     *http2.Server
	grpcListener    *chanListener
	initialSettings []http2.Setting
}

// ConfigureServer configures srv to identify gRPC connections and send them
// to the returned net.Listener, suitable for passing to grpc.Server.Serve,
// while all other HTTP requests will be handled by srv.
//
// ConfigureServer works with or without TLS enabled.
//
// When TLS is enabled, ConfigureServer relies on ALPN. ConfigureServer
// internally calls http2.ConfigureServer(srv, conf) to configure HTTP/2 support,
// and defines an alternative srv.TLSNextProto "h2" handler. When using TLS, the
// gRPC listener returns secure connections; the gRPC server must not also be
// configured to wrap the connection with TLS.
//
// When TLS is not enabled, ConfigureServer relies on h2c prior knowledge,
// wrapping srv.Handler. It is therefore necessary to set srv.Handler before
// calling ConfigureServer.
//
// The returned listener will be closed when srv.Shutdown is called. The
// returned listener's Addr() method does not correspond to the configured
// HTTP server's listener(s) in any way, and cannot be relied upon for forming
// a connection URL.
func ConfigureServer(srv *http.Server, conf *http2.Server) (grpcListener net.Listener, _ error) {
	if err := http2.ConfigureServer(srv, conf); err != nil {
		return nil, err
	}
	if conf == nil {
		conf = new(http2.Server)
	}
	initialSettings, err := backendInitialSettings(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to probe backend initial HTTP/2 settings: %w", err)
	}
	glis := newChanListener()
	mux := &mux{
		http2Server:     conf,
		grpcListener:    glis,
		initialSettings: initialSettings,
	}
	srv.Handler = mux.withGRPCInsecure(srv.Handler, srv.ErrorLog)
	srv.TLSNextProto[http2.NextProtoTLS] = func(srv *http.Server, conn *tls.Conn, h http.Handler) {
		err := mux.handleH2(srv, conn, h)
		if err != nil && srv.ErrorLog != nil {
			srv.ErrorLog.Printf("handleH2 (%s) returned an error: %s", conn.RemoteAddr(), err)
		}
	}
	srv.RegisterOnShutdown(func() { glis.Close() })
	return glis, nil
}

// withGRPCInsecure wraps next such that h2c (HTTP/2 Cleartext) gRPC requests
// are hijacked and sent to the gRPC listener, and all other HTTP requests are
// handled by next.
//
// See https://httpwg.org/specs/rfc7540.html#rfc.section.3.4
func (m *mux) withGRPCInsecure(next http.Handler, errLog *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil && r.Method == "PRI" && len(r.Header) == 0 && r.URL.Path == "*" && r.Proto == "HTTP/2.0" {
			hijacker, ok := w.(http.Hijacker)
			if ok {
				conn, rw, err := hijacker.Hijack()
				if err != nil {
					panic(fmt.Sprintf("Hijack failed: %v", err))
				}
				defer conn.Close()

				// We just identify that we're dealing with a
				// prior-knowledge connection, and pass it straight
				// through to the gRPC server.
				preface := "PRI * HTTP/2.0\r\n\r\n"
				r := io.MultiReader(strings.NewReader(preface), rw, conn)
				pc, closed := newProxyConn(conn, r, conn)
				err = m.handleGRPC(nil, pc, closed, nil)
				if err != nil && errLog != nil {
					errLog.Printf("h2c handleGRPC (%s) returned an error: %s", conn.RemoteAddr(), err)
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (m *mux) handleH2(srv *http.Server, conn net.Conn, handler http.Handler) error {
	var clientReadBuf bytes.Buffer
	connHandler, err := m.getConnHandler(conn, &clientReadBuf)
	if err != nil {
		return err
	}
	// Write frames from the server to a pipe so that we can de-duplicate the first
	// SETTINGS ACK. The client's first SETTINGS is handled twice: once in getConnHandler,
	// and once in the final connHandler. As getConnHandler will ACK the SETTINGS,
	// we need to filter out the second one ade by the final connHandler.
	rpipe, wpipe := io.Pipe()
	go func() {
		if err := copyFramesUntilSettingsAck(conn, rpipe); err != nil {
			rpipe.CloseWithError(err)
			return
		}
		_, err := io.Copy(conn, rpipe)
		rpipe.CloseWithError(err)
	}()
	proxyConn, closed := newProxyConn(conn, io.MultiReader(&clientReadBuf, conn), wpipe)
	err = connHandler(srv, proxyConn, closed, handler)
	wpipe.CloseWithError(err)
	return err
}

// copyFramesUntilSettingsAck copies http/2 frames from r to w
// up until (but excluding) the first settings ACK frame.
func copyFramesUntilSettingsAck(w io.Writer, r io.Reader) error {
	var frameBuf bytes.Buffer
	framer := http2.NewFramer(w, io.TeeReader(r, &frameBuf))
	framer.SetReuseFrames()
	var haveFirstSettingsACK bool
	for !haveFirstSettingsACK {
		f, err := framer.ReadFrame()
		if err != nil {
			return err
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !haveFirstSettingsACK && f.IsAck() {
				haveFirstSettingsACK = true
				frameBuf.Truncate(frameBuf.Len() - int(f.Length) - http2FrameHeaderLength)
				break
			}
		}
	}
	_, err := io.Copy(w, &frameBuf)
	return err
}

// getConnHandler handles a new client connection, writing a SETTINGS
// request to the client, followed by reading the HTTP/2 client preface,
// and then finally looking for a Content-Type header to determine which
// connection handler to return.
//
// All data read from the client will be written to buf, which will be
// replayed to the backend HTTP/2 server.
func (m *mux) getConnHandler(conn net.Conn, buf *bytes.Buffer) (connHandlerFunc, error) {
	rbuf := io.TeeReader(conn, buf)
	framer := http2.NewFramer(conn, rbuf)
	framer.SetReuseFrames()

	// Client expects SETTINGS first, so send initial settings.
	//
	// Mirror backend initial SETTINGS to keep gmux's sniffing SETTINGS consistent
	// with what the backend http2.Server will advertise on the same connection.
	//
	// See: https://github.com/golang/go/issues/77947
	// The real server will send a new one with the real settings.
	//
	// When replaying frames to the real server, we'll need to suppress
	// the ACK for this frame, which the server won't know about.
	if err := framer.WriteSettings(m.initialSettings...); err != nil {
		return nil, err
	}

	// Read client preface. We don't bother verifying it here, as it will
	// be verified later by the real http2.Server.
	var preface [len(http2.ClientPreface)]byte
	if _, err := io.ReadFull(rbuf, preface[:]); err != nil {
		return nil, err
	}

	contentType, err := m.getContentType(framer, buf)
	if err != nil {
		return nil, err
	}
	connHandler := m.handleHTTP
	if contentType == grpcContentType {
		connHandler = m.handleGRPC
	}
	return connHandler, nil
}

// backendInitialSettings probes backend http2 server configuration and returns
// the initial SETTINGS values emitted by http2.Server.ServeConn.
func backendInitialSettings(conf *http2.Server) (_ []http2.Setting, retErr error) {
	serverConn, clientConn := net.Pipe()
	serverDone := make(chan struct{})
	clientWriteDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		conf.ServeConn(serverConn, &http2.ServeConnOpts{
			BaseConfig: &http.Server{},
			Handler:    http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		})
	}()
	go func() {
		// Complete enough of the client-side handshake so ServeConn does not
		// emit a preface-read error during probe teardown.
		if _, err := io.WriteString(clientConn, http2.ClientPreface); err != nil {
			clientWriteDone <- err
			return
		}
		writeFramer := http2.NewFramer(clientConn, nil)
		clientWriteDone <- writeFramer.WriteSettings()
	}()
	defer func() {
		var deferErrs []error
		// Teardown everything to ensure no goroutines are left behind
		select {
		case err := <-clientWriteDone:
			if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
				deferErrs = append(deferErrs, fmt.Errorf("writing probe client preface/settings: %w", err))
			}
		case <-time.After(backendInitialSettingsTimeout):
			deferErrs = append(deferErrs, fmt.Errorf("probe client write timeout"))
		}
		if err := serverConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			deferErrs = append(deferErrs, fmt.Errorf("closing probe server pipe: %w", err))
		}
		if err := clientConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			deferErrs = append(deferErrs, fmt.Errorf("closing probe client pipe: %w", err))
		}
		select {
		case <-serverDone:
		case <-time.After(backendInitialSettingsTimeout): // Unexpected for conf.ServeConn to not return; fail fast
			deferErrs = append(deferErrs, fmt.Errorf("serve conn timeout"))
		}

		if len(deferErrs) > 0 {
			retErr = errors.Join(append([]error{retErr}, deferErrs...)...)
		}
	}()

	if err := clientConn.SetReadDeadline(time.Now().Add(backendInitialSettingsTimeout)); err != nil {
		return nil, fmt.Errorf("setting probe read deadline: %w", err)
	}
	framer := http2.NewFramer(nil, clientConn)
	frame, err := framer.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("reading initial settings frame: %w", err)
	}
	settingsFrame, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return nil, fmt.Errorf("unexpected first frame type %T", frame)
	}

	var settings []http2.Setting
	if err := settingsFrame.ForeachSetting(func(s http2.Setting) error {
		settings = append(settings, s)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("decoding initial settings: %w", err)
	}
	return settings, nil
}

var decoderPool = sync.Pool{
	New: func() interface{} {
		out := &decoder{}
		out.d = hpack.NewDecoder(4096, func(hf hpack.HeaderField) {
			if hf.Name == "content-type" {
				out.contentType = hf.Value
			}
		})
		return out
	},
}

type decoder struct {
	d           *hpack.Decoder
	contentType string
}

func (m *mux) getContentType(framer *http2.Framer, framesBuf *bytes.Buffer) (contentType string, _ error) {
	// Code based on https://github.com/soheilhy/cmux
	//
	// Copyright 2016 The CMux Authors. All rights reserved.
	dec := decoderPool.Get().(*decoder)

	// Read frames until we have the content-type header, or we know there isn't one.
	var haveFirstSettings bool
	var haveFirstSettingsACK bool
	var haveEndHeaders bool
	for (dec.contentType == "" && !haveEndHeaders) || !haveFirstSettings || !haveFirstSettingsACK {
		f, err := framer.ReadFrame()
		if err != nil {
			return "", err
		}

		switch f := f.(type) {
		case *http2.SettingsFrame:
			switch {
			case !haveFirstSettingsACK && f.IsAck():
				haveFirstSettingsACK = true
				// We accept the ACK, and omit it from the frames
				// written to the real server.
				framesBuf.Truncate(framesBuf.Len() - int(f.Length) - http2FrameHeaderLength)
			case !haveFirstSettings && !f.IsAck():
				haveFirstSettings = true
				// We ACK the client's first SETTINGS to unblock it,
				// and ignore the first ACK from the real server.
				if err := framer.WriteSettingsAck(); err != nil {
					return "", err
				}
			}
		case *http2.ContinuationFrame:
			if _, err := dec.d.Write(f.HeaderBlockFragment()); err != nil {
				return "", err
			}
			haveEndHeaders = f.FrameHeader.Flags&http2.FlagHeadersEndHeaders != 0
		case *http2.HeadersFrame:
			if _, err := dec.d.Write(f.HeaderBlockFragment()); err != nil {
				return "", err
			}
			haveEndHeaders = f.FrameHeader.Flags&http2.FlagHeadersEndHeaders != 0
		}
	}
	contentType = dec.contentType
	if dec.d.Close() == nil {
		dec.contentType = ""
		decoderPool.Put(dec)
	}
	return contentType, nil
}

type connHandlerFunc func(srv *http.Server, conn net.Conn, closed <-chan struct{}, handler http.Handler) error

func (m *mux) handleHTTP(srv *http.Server, conn net.Conn, closed <-chan struct{}, handler http.Handler) error {
	// This code is adapted from x/net/http2 to not assume tls.Conn.

	// The TLSNextProto interface predates contexts, so the net/http package passes
	// down its per-connection base context via an exported but unadvertised method
	// on the Handler. This is for internal net/http<=>http2 use only.
	var ctx context.Context
	type baseContexter interface {
		BaseContext() context.Context
	}
	if bc, ok := handler.(baseContexter); ok {
		ctx = bc.BaseContext()
	}
	m.http2Server.ServeConn(conn, &http2.ServeConnOpts{
		Context:    ctx,
		Handler:    handler,
		BaseConfig: srv,
	})
	return nil
}

func (m *mux) handleGRPC(_ *http.Server, conn net.Conn, closed <-chan struct{}, _ http.Handler) error {
	select {
	case <-m.grpcListener.closed:
		return errors.New("grpc listener closed")
	case m.grpcListener.conns <- conn:
	case <-closed:
		// Connection closed before it could be handled.
		return nil
	}
	<-closed
	return nil
}
