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
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
)

func newProxyConn(raw net.Conn, r io.Reader, w io.Writer) (_ net.Conn, closed <-chan struct{}) {
	pc := proxyConn{Conn: raw, closed: make(chan struct{}), r: r, w: w}
	if raw, ok := raw.(*tls.Conn); ok {
		return &tlsProxyConn{connectionStater: raw, proxyConn: pc}, pc.closed
	}
	return &pc, pc.closed
}

type tlsProxyConn struct {
	connectionStater
	proxyConn
}

type connectionStater interface {
	ConnectionState() tls.ConnectionState
}

func (c *tlsProxyConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func (c *tlsProxyConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func (c *tlsProxyConn) Write(b []byte) (int, error) {
	return c.w.Write(b)
}

type proxyConn struct {
	net.Conn
	closeOnce sync.Once
	closed    chan struct{}
	r         io.Reader
	w         io.Writer
}

func (c *proxyConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func (c *proxyConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func (c *proxyConn) Write(b []byte) (int, error) {
	return c.w.Write(b)
}

type chanListener struct {
	closeOnce sync.Once
	closed    chan struct{}
	conns     chan net.Conn
}

func newChanListener() *chanListener {
	return &chanListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *chanListener) Addr() net.Addr {
	return gmuxAddr{}
}

func (l *chanListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, errors.New("listener closed")
	case conn := <-l.conns:
		return conn, nil
	}
}

type gmuxAddr struct{}

func (gmuxAddr) Network() string {
	return "gmux"
}

func (gmuxAddr) String() string {
	return "gmux"
}
