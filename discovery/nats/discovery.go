// MIT License
//
// Copyright (c) 2025-2026 Arsene Tochemey Gandote
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package nats

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/flowchartsman/retry"
	"github.com/nats-io/nats.go"
	"go.uber.org/atomic"

	"github.com/tochemey/distcache/discovery"
	"github.com/tochemey/distcache/internal/validation"
	"github.com/tochemey/distcache/log"
)

// replyChannelSize defines the buffer size of the inbox channel used
// to collect peers responses. nats.go drops messages when the channel
// is full, so the buffer is generous on purpose.
const replyChannelSize = 1024

// Discovery represents the kubernetes discovery
type Discovery struct {
	config *Config
	mu     sync.Mutex

	initialized *atomic.Bool
	registered  *atomic.Bool

	// define the nats connection
	connection *nats.Conn
	// define a slice of subscriptions
	subscriptions []*nats.Subscription

	address string
	// define a logger
	logger log.Logger
}

// enforce compilation error
var _ discovery.Provider = &Discovery{}

// NewDiscovery returns an instance of the kubernetes discovery provider
func NewDiscovery(config *Config) *Discovery {
	// create an instance of
	discovery := &Discovery{
		mu:          sync.Mutex{},
		initialized: atomic.NewBool(false),
		registered:  atomic.NewBool(false),
		logger:      log.New(log.InfoLevel, os.Stdout),
		config:      config,
	}

	discovery.address = net.JoinHostPort(config.Host, strconv.Itoa(int(config.DiscoveryPort)))
	return discovery
}

// ID returns the discovery provider id
func (d *Discovery) ID() string {
	return "nats"
}

// Initialize initializes the plugin: registers some internal data structures, clients etc.
func (d *Discovery) Initialize() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.initialized.Load() {
		return discovery.ErrAlreadyInitialized
	}

	if err := d.config.Validate(); err != nil {
		return err
	}

	if d.config.Timeout <= 0 {
		d.config.Timeout = time.Second
	}

	if d.config.MaxJoinAttempts == 0 {
		d.config.MaxJoinAttempts = 5
	}

	if d.config.ReconnectWait <= 0 {
		d.config.ReconnectWait = 2 * time.Second
	}

	// create the nats connection option
	opts := nats.GetDefaultOptions()
	opts.Url = d.config.Server
	opts.Name = net.JoinHostPort(d.config.Host, strconv.Itoa(int(d.config.DiscoveryPort)))
	opts.ReconnectWait = d.config.ReconnectWait
	opts.MaxReconnect = -1

	var connection *nats.Conn

	// let us connect using an exponential backoff mechanism
	// create a new instance of retrier that will try a maximum of MaxJoinAttempts times, with
	// an initial delay of 100 ms and a maximum delay of opts.ReconnectWait
	err := retry.
		NewRetrier(d.config.MaxJoinAttempts, 100*time.Millisecond, opts.ReconnectWait).
		Run(func() error {
			var err error
			connection, err = opts.Connect()
			return err
		})

	if err != nil {
		return fmt.Errorf("failed to connect to the NATS server %s: %w", d.config.Server, err)
	}

	if connection == nil {
		return fmt.Errorf("failed to connect to the NATS server %s", d.config.Server)
	}

	// create the NATs connection
	d.connection = connection
	d.initialized.Store(true)
	return nil
}

// Register registers this node to a service discovery directory.
func (d *Discovery) Register() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.registered.Load() {
		return discovery.ErrAlreadyRegistered
	}

	if d.connection == nil {
		return discovery.ErrNotInitialized
	}

	// create the subscription handler
	handler := func(msg *nats.Msg) {
		message := new(Message)
		if err := json.Unmarshal(msg.Data, message); err != nil {
			// TODO: need to read more and see how to propagate the error
			d.connection.Opts.AsyncErrorCB(d.connection, msg.Sub, errors.New("nats: Got an error trying to unmarshal: "+err.Error()))
			return
		}

		switch message.Type {
		case Deregister:
			// pass
		case Register:
			// pass
		case Request:
			response := &Message{
				Host: d.config.Host,
				Port: d.config.DiscoveryPort,
				Name: net.JoinHostPort(d.config.Host, strconv.Itoa(int(d.config.DiscoveryPort))),
				Type: Response,
			}

			bytea, _ := json.Marshal(response)
			if err := d.connection.Publish(msg.Reply, bytea); err != nil {
				d.logger.Error(err)
			}
		}
	}

	subscription, err := d.connection.Subscribe(d.config.Subject, handler)
	if err != nil {
		return err
	}

	// make sure the subscription is active on the server before reporting success,
	// otherwise peers requests published right after Register could be missed
	if err := d.connection.Flush(); err != nil {
		_ = subscription.Unsubscribe()
		return fmt.Errorf("failed to flush the NATS subscription: %w", err)
	}

	d.subscriptions = append(d.subscriptions, subscription)
	d.registered.Store(true)
	return nil
}

// Deregister removes this node from a service discovery directory.
func (d *Discovery) Deregister() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// first check whether the discovery provider has been registered or not
	if !d.registered.Load() {
		return discovery.ErrNotRegistered
	}

	// shutdown all the subscriptions
	for _, subscription := range d.subscriptions {
		// when subscription is defined
		if subscription != nil {
			// check whether the subscription is active or not
			if subscription.IsValid() {
				// unsubscribe and return when there is an error
				if err := subscription.Unsubscribe(); err != nil {
					return err
				}
			}
		}
	}

	var err error
	// send the de-registration message to notify peers
	if d.connection != nil {
		// send a message to deregister stating we are out
		message := &Message{
			Host: d.config.Host,
			Port: d.config.DiscoveryPort,
			Name: net.JoinHostPort(d.config.Host, strconv.Itoa(int(d.config.DiscoveryPort))),
			Type: Deregister,
		}

		bytea, _ := json.Marshal(message)
		err = d.connection.Publish(d.config.Subject, bytea)
	}

	// reset the registration state regardless of the publish outcome
	// so that a subsequent Register call can succeed
	d.subscriptions = nil
	d.registered.Store(false)
	return err
}

// DiscoverPeers returns a list of known nodes.
func (d *Discovery) DiscoverPeers() ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized.Load() {
		return nil, discovery.ErrNotInitialized
	}

	if !d.registered.Load() {
		return nil, discovery.ErrNotRegistered
	}

	// Set up a reply channel, then broadcast for all peers to
	// report their presence.
	// collect as many responses as possible in the given timeout.
	inbox := nats.NewInbox()
	msgCh := make(chan *nats.Msg, replyChannelSize)

	// bind to receive messages
	sub, err := d.connection.ChanSubscribe(inbox, msgCh)
	if err != nil {
		return nil, err
	}

	// make sure the inbox subscription never leaks whatever the exit path
	defer func() {
		_ = sub.Unsubscribe()
	}()

	request := &Message{
		Host: d.config.Host,
		Port: d.config.DiscoveryPort,
		Name: net.JoinHostPort(d.config.Host, strconv.Itoa(int(d.config.DiscoveryPort))),
		Type: Request,
	}

	bytea, _ := json.Marshal(request)
	if err = d.connection.PublishRequest(d.config.Subject, inbox, bytea); err != nil {
		return nil, err
	}

	var peers []string
	seen := make(map[string]struct{})
	timeout := time.After(d.config.Timeout)
	me := net.JoinHostPort(d.config.Host, strconv.Itoa(int(d.config.DiscoveryPort)))
	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				// Subscription is closed
				return peers, nil
			}

			message := new(Message)
			if err := json.Unmarshal(msg.Data, message); err != nil {
				return nil, err
			}

			// get the found peer address
			addr := net.JoinHostPort(message.Host, strconv.Itoa(int(message.Port)))
			if addr == me {
				continue
			}

			// never trust a peer-supplied address blindly
			if err := validation.NewTCPAddressValidator(addr).Validate(); err != nil {
				d.logger.Warnf("skipping invalid peer address=(%s): %v", addr, err)
				continue
			}

			// dedupe
			if _, exists := seen[addr]; exists {
				continue
			}

			seen[addr] = struct{}{}
			peers = append(peers, addr)

		case <-timeout:
			return peers, nil
		}
	}
}

// Close closes the provider
func (d *Discovery) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.initialized.Store(false)
	d.registered.Store(false)

	if d.connection != nil {
		defer func() {
			d.connection.Close()
			d.connection = nil
		}()

		for _, subscription := range d.subscriptions {
			if subscription != nil {
				if subscription.IsValid() {
					if err := subscription.Unsubscribe(); err != nil {
						return err
					}
				}
			}
		}

		d.subscriptions = nil
		return d.connection.Flush()
	}
	return nil
}
