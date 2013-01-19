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
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net"
	"strings"
)

var (
	keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
)

func headerHasValue(header map[string][]string, key string, value string) bool {
	for _, v := range header[key] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(value, strings.TrimSpace(s)) {
				return true
			}
		}
	}
	return false
}

// Upgrade upgrades the HTTP server connection to the WebSocket protocol. The
// resp argument is any object that suports the http.Hijack interface
// (http.ResponseWriter, Twister web.Responder, Indigo web.Responder).
func Upgrade(resp interface{}, header map[string][]string, subProtocol string, readBufSize, writeBufSize int) (*Conn, error) {

	if values := header["Sec-Websocket-Version"]; len(values) == 0 || values[0] != "13" {
		return nil, errors.New("websocket: version != 13")
	}

	if !headerHasValue(header, "Connection", "upgrade") {
		return nil, errors.New("websocket: connection header != upgrade")
	}

	if !headerHasValue(header, "Upgrade", "websocket") {
		return nil, errors.New("websocket: upgrade != websocket")
	}

	var key []byte
	if values := header["Sec-Websocket-Key"]; len(values) == 0 || values[0] == "" {
		return nil, errors.New("websocket: key missing or blank")
	} else {
		key = []byte(values[0])
	}

	h := sha1.New()
	h.Write(key)
	h.Write(keyGUID)
	accpektKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	var buf bytes.Buffer
	buf.WriteString("HTTP/1.1 101 Switching Protocols")
	buf.WriteString("\r\nUpgrade: websocket")
	buf.WriteString("\r\nConnection: Upgrade")
	buf.WriteString("\r\nSec-WebSocket-Accept: ")
	buf.WriteString(accpektKey)
	if subProtocol != "" {
		buf.WriteString("\r\nSec-WebSocket-Protocol: ")
		buf.WriteString(subProtocol)
	}
	buf.WriteString("\r\n\r\n")

	var netConn net.Conn
	var br *bufio.Reader
	var err error

	if h, ok := resp.(interface {
		Hijack() (net.Conn, *bufio.Reader, error)
	}); ok {
		// Indigo, Twister
		netConn, br, err = h.Hijack()
	} else if h, ok := resp.(interface {
		Hijack() (net.Conn, *bufio.ReadWriter, error)
	}); ok {
		// Standard HTTP package.
		var rw *bufio.ReadWriter
		netConn, rw, err = h.Hijack()
		br = rw.Reader
	} else {
		return nil, errors.New("websocket: resp does not support Hijack")
	}

	if br.Buffered() > 0 {
		return nil, errors.New("websocket: client sent data before handshake complete")
	}

	if _, err = netConn.Write(buf.Bytes()); err != nil {
		netConn.Close()
		return nil, err
	}

	return newConn(netConn, true, readBufSize, writeBufSize), nil
}
