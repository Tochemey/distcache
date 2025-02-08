/*
 * MIT License
 *
 * Copyright (c) 2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package distcache

import (
	"encoding/json"
	"net"
	"strconv"
)

// Peer defines the discovery peer
type Peer struct {
	// BindAddr denotes the address that Engine will bind to for communication
	// with other Engine nodes.
	BindAddr string `json:"bind_addr"`

	// BindPort denotes the address that Engine will bind to for communication
	// with other Engine nodes.
	BindPort int `json:"bind_port"`

	// DiscoveryPort denotes the port engine will use to discover other engine nodes
	// in the cluster
	DiscoveryPort int `json:"discovery_port"`

	// IsSelf denotes whether the given peer is the running node
	IsSelf bool `json:"is_self"`
}

func (p Peer) Address() string {
	return net.JoinHostPort(p.BindAddr, strconv.Itoa(p.BindPort))
}

// fromBytes returns the instance of Peer from bytea array
func fromBytes(bytes []byte) (*Peer, error) {
	peer := &Peer{}
	err := json.Unmarshal(bytes, peer)
	return peer, err
}
