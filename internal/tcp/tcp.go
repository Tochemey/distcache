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

package tcp

import (
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/hashicorp/go-sockaddr"
)

// GetHostPort returns the actual ip address and port from a given address
func GetHostPort(address string) (string, int, error) {
	// Get the address
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return "", 0, err
	}

	return addr.IP.String(), addr.Port, nil
}

// GetBindIP tries to find an appropriate bindIP to bind and propagate.
func GetBindIP(ifname, address string) (string, error) {
	bindIP, _, err := GetHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid BindAddr: %w", err)
	}

	// Check if we have an interface
	if iface, _ := net.InterfaceByName(ifname); iface != nil {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", fmt.Errorf("failed to get interface addresses: %w", err)
		}

		if len(addrs) == 0 {
			return "", fmt.Errorf("interface '%s' has no addresses", ifname)
		}

		// If there is no bind IP, pick an address
		if bindIP == "0.0.0.0" {
			addr, err := getBindIPFromNetworkInterface(addrs)
			if err != nil {
				return "", fmt.Errorf("ip scan on %s: %w", ifname, err)
			}
			return addr, nil
		}

		// If there is a bind IP, ensure it is available
		for _, a := range addrs {
			addr, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if addr.IP.String() == bindIP {
				return bindIP, nil
			}
		}
		return "", fmt.Errorf("interface '%s' has no '%s' address", ifname, bindIP)
	}

	if bindIP == "0.0.0.0" {
		// if we're not bound to a specific IP, let's use a suitable private IP address.
		ipStr, err := sockaddr.GetPrivateIP()
		if err != nil {
			return "", fmt.Errorf("failed to get private interface addresses: %w", err)
		}

		// if we could not find a private address, we need to expand our search to a public
		// ip address
		if ipStr == "" {
			ipStr, err = sockaddr.GetPublicIP()
			if err != nil {
				return "", fmt.Errorf("failed to get public interface addresses: %w", err)
			}
		}

		if ipStr == "" {
			return "", fmt.Errorf("no private IP address found, and explicit IP not provided")
		}

		parsed := net.ParseIP(ipStr)
		if parsed == nil {
			return "", fmt.Errorf("failed to parse private IP address: %q", ipStr)
		}
		bindIP = parsed.String()
	}
	return bindIP, nil
}

// KeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by Serve so that dead TCP connections eventually
// go away.
type KeepAliveListener struct {
	*net.TCPListener
}

func (ln KeepAliveListener) Accept() (c net.Conn, err error) {
	if c, err = ln.AcceptTCP(); err != nil {
		return
	} else if err = c.(*net.TCPConn).SetKeepAlive(true); err != nil {
		return
	}
	// Ignore error from setting the KeepAlivePeriod as some systems, such as
	// OpenBSD, do not support setting TCP_USER_TIMEOUT on IPPROTO_TCP
	_ = c.(*net.TCPConn).SetKeepAlivePeriod(3 * time.Minute)
	return
}

// NewKeepAliveListener creates an instance of KeepAliveListener
func NewKeepAliveListener(address string) (*KeepAliveListener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &KeepAliveListener{listener.(*net.TCPListener)}, nil
}

func getBindIPFromNetworkInterface(addrs []net.Addr) (string, error) {
	for _, a := range addrs {
		var addrIP net.IP
		switch runtime.GOOS {
		case "windows":
			addr, ok := a.(*net.IPAddr)
			if !ok {
				continue
			}
			addrIP = addr.IP
		default:
			addr, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			addrIP = addr.IP
		}

		// Skip self-assigned IPs
		if addrIP.IsLinkLocalUnicast() {
			continue
		}
		return addrIP.String(), nil
	}
	return "", fmt.Errorf("failed to find usable address for interface")
}
