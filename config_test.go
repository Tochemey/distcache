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
	"fmt"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/distcache/discovery/nats"
	"github.com/tochemey/distcache/internal/size"
)

func TestConfig(t *testing.T) {
	t.Run("WithValid config", func(t *testing.T) {
		// start the NATS server
		srv := StartNatsServer(t)
		serverAddress := srv.Addr().String()
		// generate the ports for the single startNode
		ports := dynaport.Get(1)
		discoveryPort := ports[0]
		host := "127.0.0.1"

		// create the nats discovery provider
		provider := nats.NewDiscovery(&nats.Config{
			Server:        fmt.Sprintf("nats://%s", serverAddress),
			Subject:       "example",
			Host:          host,
			DiscoveryPort: discoveryPort,
		})

		dataSource := NewMockDataSource()
		keySpaces := []KeySpace{
			NewMockKeySpace("users", size.MB, dataSource),
		}

		config := NewConfig(provider, keySpaces)
		err := config.Validate()
		require.NoError(t, err)

		err = provider.Close()
		require.NoError(t, err)
		srv.Shutdown()
	})
}

func StartNatsServer(t *testing.T) *natsserver.Server {
	t.Helper()
	serv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1",
		Port: -1,
	})

	require.NoError(t, err)

	ready := make(chan bool)
	go func() {
		ready <- true
		serv.Start()
	}()
	<-ready

	if !serv.ReadyForConnections(2 * time.Second) {
		t.Fatalf("nats-io server failed to start")
	}

	return serv
}
