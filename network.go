// Copyright 2024 Anapaya Systems
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scion

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"
	snetmetrics "github.com/scionproto/scion/pkg/snet/metrics"
	"github.com/scionproto/scion/pkg/sock/reliable"
	"github.com/scionproto/scion/pkg/sock/reliable/reconnect"
	"github.com/scionproto/scion/private/app/env"
	"go.uber.org/zap"
)

var (
	_ caddy.ListenerFunc = (*Network)(nil).Listen

	_ net.Listener   = (*blockedListener)(nil)
	_ net.PacketConn = (*conn)(nil)
)

var (
	globalNetwork = Network{
		connPool: NewUsagePool[string, *conn](),
		nets:     map[addr.IA]*snet.SCIONNetwork{},
	}

	envFile = func() string {
		if file := os.Getenv("SCION_ENV_FILE"); file != "" {
			return file
		}
		return "/etc/scion/environment.json"
	}()

	metrics = snetmetrics.NewSCIONPacketConnMetrics()
)

// Network is a custom network that allows to listen on SCION addresses.
type Network struct {
	connPool *UsagePool[string, *conn]

	netsMtx sync.Mutex
	nets    map[addr.IA]*snet.SCIONNetwork

	logger atomic.Pointer[zap.Logger]
}

// SetLogger sets the logger for the network. It is safe to access concurrently.
func (n *Network) SetLogger(logger *zap.Logger) {
	n.logger.Store(logger)
}

// Logger gets the logger.
func (n *Network) Logger() *zap.Logger {
	return n.logger.Load()
}

// ListenBlocked returns a blocked net.Listener that will never accept a
// connection. It is used to fake a net.Listener for caddyhttp HTTP1.1 and
// HTTP2. It is required such that we can create a HTTP3 listener.
func (n *Network) ListenBlocked(
	ctx context.Context,
	network string,
	address string,
	cfg net.ListenConfig,
) (any, error) {
	if network != "scion" {
		return nil, fmt.Errorf("network not supported: %s", network)
	}
	conn, err := n.listen(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	return &blockedListener{conn}, nil
}

// Listen returns a net.PacketConn that listens on the given address. It is used
// by caddyhttp to create a HTTP3 listener.
func (n *Network) Listen(
	ctx context.Context,
	network string,
	address string,
	cfg net.ListenConfig,
) (any, error) {
	if network != "scion+quic" {
		return nil, fmt.Errorf("network not supported: %s", network)
	}
	conn, err := n.listen(ctx, address, cfg)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// listen listens on the given address. If there is already a connection for
// that address, it is returned. Otherwise a new connection is created. This is
// required because caddy binds to the same address multiple times during
// configuration changes.
func (n *Network) listen(ctx context.Context, address string, cfg net.ListenConfig) (*conn, error) {
	listen, err := snet.ParseUDPAddr(address)
	if err != nil {
		return nil, fmt.Errorf("parsing listening address: %w", err)
	}
	if listen.Host.Port == 0 {
		return nil, fmt.Errorf("wildcard port not supported: %s", address)
	}
	env, err := n.loadEnv()
	if err != nil {
		return nil, fmt.Errorf("loading environment configuration data: %w", err)
	}
	if _, ok := env.ASes[listen.IA]; !ok {
		return nil, fmt.Errorf(
			"listening address (%s) not covered by configured ASes %q",
			address,
			env.ASes,
		)
	}

	network := func() *snet.SCIONNetwork {
		n.netsMtx.Lock()
		defer n.netsMtx.Unlock()
		if net, ok := n.nets[listen.IA]; ok {
			return net
		}
		net := &snet.SCIONNetwork{
			LocalIA: listen.IA,
			Dispatcher: &snet.DefaultPacketDispatcherService{
				Dispatcher: reconnect.NewDispatcherService(
					reliable.NewDispatcher(env.General.DispatcherSocket),
				),
				SCMPHandler:            ignoreSCMP{},
				SCIONPacketConnMetrics: metrics,
			},
		}
		n.nets[listen.IA] = net
		return net
	}()

	c, loaded, err := n.connPool.LoadOrNew(address, func() (caddy.Destructor, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		c, err := network.Listen(ctx, "udp", listen.Host, addr.SvcNone)
		if err != nil {
			return nil, err
		}
		return &conn{
			PacketConn: c,
			addr:       address,
			network:    n,
			closed:     make(chan struct{}),
		}, nil
	})
	if err != nil {
		return nil, err
	}
	n.Logger().Debug("created new listener", zap.String("addr", address), zap.Bool("reuse", loaded))
	return c, err
}

func (n *Network) loadEnv() (env.SCION, error) {
	raw, err := os.ReadFile(envFile)
	if err != nil {
		return env.SCION{}, err
	}
	var e env.SCION
	if err := json.Unmarshal(raw, &e); err != nil {
		return env.SCION{}, err
	}
	return e, nil
}

type conn struct {
	net.PacketConn
	addr    string
	network *Network
	closed  chan struct{}
}

// Close removes the reference in the usage pool. If the references go to zero,
// the connection is destroyed.
func (c *conn) Close() error {
	_, err := c.network.connPool.Delete(c.addr)
	return err
}

// Destruct closes the connection. It is called by the usage pool when the
// reference count goes to zero.
func (c *conn) Destruct() error {
	c.network.Logger().Debug("destroying listener", zap.String("addr", c.addr))
	defer c.network.Logger().Debug("destroyed listener", zap.String("addr", c.addr))

	close(c.closed)
	return c.PacketConn.Close()
}

// blockedListener is a net.Listener that will never accept a connection. It
// blocks until the underlying connection is closed.
type blockedListener struct {
	*conn
}

func (l *blockedListener) Accept() (net.Conn, error) {
	l.network.Logger().Debug("start accepting on blocked listener", zap.String("addr", l.addr))
	defer l.network.Logger().
		Debug("stopped accepting on blocked listener", zap.String("addr", l.addr))

	<-l.conn.closed
	return nil, fmt.Errorf("listener closed")
}

func (l *blockedListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// ignoreSCMP is a SCMP handler that ignores all SCMP messages. This is required
// because SCMP error messages should not close the accept loop.
type ignoreSCMP struct{}

func (ignoreSCMP) Handle(pkt *snet.Packet) error {
	// Always reattempt reads from the socket.
	return nil
}
