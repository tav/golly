// Changes to this file by The Golly Authors are in the Public Domain.
// See the Golly UNLICENSE file for details.

// Copyright 2011 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrInvalidConnectionHeader = errors.New("websocket: connection header != upgrade")
	ErrInvalidHandshake        = errors.New("websocket: client sent data before handshake complete")
	ErrInvalidUpgradeHeader    = errors.New("websocket: upgrade != websocket")
	ErrMissingKeyHeader        = errors.New("websocket: key missing or blank")
	ErrMismatchingOrigin       = errors.New("websocket: origin does not match")
)

var (
	keyGUID        = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
	responseHeader = []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ")
)

// Upgrade upgrades the HTTP server connection to the WebSocket protocol.
func Upgrade(w http.ResponseWriter, r *http.Request, origin, subProtocol string) (*Conn, error, bool) {

	wc, buf, err := w.(http.Hijacker).Hijack()
	if err != nil {
		panic("websocket: hijack failed: " + err.Error())
	}

	if version := r.Header.Get("Sec-Websocket-Version"); version != "13" {
		return nil, fmt.Errorf("websocket: unsupported version %q", version), false
	}

	if strings.ToLower(r.Header.Get("Connection")) != "upgrade" {
		return nil, ErrInvalidConnectionHeader, false
	}

	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		return nil, ErrInvalidUpgradeHeader, false
	}

	if origin != "" && origin != r.Header.Get("Origin") {
		return nil, ErrMismatchingOrigin, false
	}

	key := []byte(r.Header.Get("Sec-Websocket-Key"))
	if len(key) == 0 {
		return nil, ErrMissingKeyHeader, false
	}

	h := sha1.New()
	h.Write(key)
	h.Write(keyGUID)

	resp := make([]byte, 28)
	base64.StdEncoding.Encode(resp, h.Sum(nil))
	if subProtocol != "" {
		resp = append(resp, "\r\nSec-WebSocket-Protocol: "...)
		resp = append(resp, subProtocol...)
	}
	resp = append(resp, "\r\n\r\n"...)

	if buf.Reader.Buffered() > 0 {
		return nil, ErrInvalidHandshake, false
	}

	if _, err = buf.Write(responseHeader); err != nil {
		wc.Close()
		return nil, err, true
	}
	if _, err = buf.Write(resp); err != nil {
		wc.Close()
		return nil, err, true
	}
	if err = buf.Flush(); err != nil {
		wc.Close()
		return nil, err, true
	}

	return newConn(wc, true, 1024, 1024), nil, false

}
