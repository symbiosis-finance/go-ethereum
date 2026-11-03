// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// A peer that disconnects while a call is running must not wedge the server: the
// call's context is cancelled, the call returns, and ServeCodec ends.
func TestServerDisconnectCancelsInflightCall(t *testing.T) {
	server := newTestServer()
	defer server.Stop()

	serverConn, peerConn := net.Pipe()
	served := make(chan struct{})
	go func() {
		server.ServeCodec(NewCodec(serverConn), 0)
		close(served)
	}()

	// test_block returns only when its context ends.
	if _, err := peerConn.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"test_block"}`)); err != nil {
		t.Fatal(err)
	}
	peerConn.Close()

	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("ServeCodec did not return after the peer disconnected: the in-flight call was never cancelled")
	}
}

var errWriteFailed = errors.New("write failed")

// failingWriteCodec is a server codec whose responses never get through.
type failingWriteCodec struct {
	ServerCodec
}

func (c failingWriteCodec) writeJSON(context.Context, interface{}, bool) error {
	return errWriteFailed
}

// A failed response write closes the connection, so the peer sees it end instead
// of waiting for a reply that cannot arrive.
func TestServerClosesConnectionOnFailedWrite(t *testing.T) {
	server := newTestServer()
	defer server.Stop()

	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()
	codec := failingWriteCodec{NewCodec(serverConn)}
	go server.ServeCodec(codec, 0)

	if _, err := peerConn.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"test_echo","params":["x",1,{"S":"y"}]}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-codec.closed():
	case <-time.After(5 * time.Second):
		t.Fatal("the connection stayed open after a response could not be written")
	}
}
