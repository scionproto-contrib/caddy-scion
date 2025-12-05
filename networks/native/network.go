// Copyright 2024 Anapaya Systems, ETH Zurich
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

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/app/env"
	"go.uber.org/zap"

	"github.com/scionproto-contrib/caddy-scion/networks/pool"
)

var (
	_ caddy.ListenerFunc = (*Network)(nil).Listen

	_ net.PacketConn = (*conn)(nil)
)

const (
	SCIONNetwork = "scion"
	SCIONUDP     = "scion+udp"

	initTimeout = 1 * time.Second
)

// listener defines an interface for creating a QUIC listener.
// It provides a method to start listening for incoming QUIC connections.
// This interface is used to allow for testing.
type listener interface {
	listen(ctx context.Context,
		network *Network,
		laddr *snet.UDPAddr,
		cfg net.ListenConfig) (caddy.Destructor, error)
}

// Network is a custom network that allows to listen on SCION addresses.
type Network struct {
	Pool              *pool.UsagePool[string, *conn]
	PacketConnMetrics snet.SCIONPacketConnMetrics

	logger   atomic.Pointer[zap.Logger]
	listener listener
}

func NewNetwork(pool *pool.UsagePool[string, *conn]) *Network {
	return &Network{
		Pool:     pool,
		listener: &listenerSCIONUDP{},
	}
}

// SetLogger sets the logger for the network. It is safe to access concurrently.
func (n *Network) SetLogger(logger *zap.Logger) {
	n.logger.Store(logger)
}

func (n *Network) SetPacketConnMetrics(metrics snet.SCIONPacketConnMetrics) {
	n.PacketConnMetrics = metrics
}

// Logger gets the logger.
func (n *Network) Logger() *zap.Logger {
	return n.logger.Load()
}

func (n *Network) Listen(
	ctx context.Context,
	network string,
	host string,
	portRange string,
	portOffset uint,
	cfg net.ListenConfig,
) (any, error) {
	if network != SCIONUDP {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
	if strings.Contains(portRange, "-") {
		return nil, fmt.Errorf("port ranges are not supported for SCION listeners, got: %s", portRange)
	}
	// PortOffset should be 0 for single ports, we fail if not.
	if portOffset != 0 {
		return nil, fmt.Errorf("port offsets are not supported for SCION UDP listeners")
	}
	address := net.JoinHostPort(host, portRange)
	laddr, err := snet.ParseUDPAddr(address)
	if err != nil {
		return nil, fmt.Errorf("parsing listening address: %w", err)
	}
	if laddr.Host.Port == 0 {
		return nil, fmt.Errorf("wildcard port not supported: %s", address)
	}

	key := poolKey(network, laddr.String())
	c, loaded, err := n.Pool.LoadOrNew(key, func() (caddy.Destructor, error) {
		return n.listener.listen(ctx, n, laddr, cfg)
	})
	if err != nil {
		return nil, err
	}
	n.Logger().Debug("created new listener", zap.String("addr", key), zap.Bool("reuse", loaded))
	return c, nil
}

type listenerSCIONUDP struct {
}

func (l *listenerSCIONUDP) listen(
	ctx context.Context,
	network *Network,
	laddr *snet.UDPAddr,
	cfg net.ListenConfig,
) (caddy.Destructor, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	sd, err := sciondConn(laddr.IA)
	if err != nil {
		network.Logger().Error("failed to connect to SCIOND", zap.Error(err))
		return nil, err
	}

	n := &snet.SCIONNetwork{
		Topology:          sd,
		SCMPHandler:       ignoreSCMP{},
		PacketConnMetrics: network.PacketConnMetrics,
	}

	c, err := n.Listen(ctx, "udp", laddr.Host)
	if err != nil {
		network.Logger().Error("failed to listen on scion+udp", zap.Error(err))
		return nil, err
	}

	network.Logger().Debug("created new scion+udp listener", zap.String("addr", laddr.String()))
	return &conn{
		PacketConn: c,
		addr:       laddr.String(),
		network:    network,
	}, nil
}

type conn struct {
	net.PacketConn
	addr    string
	network *Network
}

// Close removes the reference in the usage pool. If the references go to zero,
// the connection is destroyed.
func (c *conn) Close() error {
	_, err := c.network.Pool.Delete(poolKey(SCIONUDP, c.addr))
	return err
}

// Destruct closes the connection. It is called by the usage pool when the
// reference count goes to zero.
func (c *conn) Destruct() error {
	c.network.Logger().Debug("destroying listener", zap.String("addr", c.addr))
	defer c.network.Logger().Debug("destroyed listener", zap.String("addr", c.addr))

	return c.PacketConn.Close()
}

// ignoreSCMP is a SCMP handler that ignores all SCMP messages. This is required
// because SCMP error messages should not close the accept loop.
type ignoreSCMP struct{}

func (ignoreSCMP) Handle(pkt *snet.Packet) error {
	// Always reattempt reads from the socket.
	return nil
}

func poolKey(network string, address string) string {
	return fmt.Sprintf("%s:%s", network, address)
}

// SCIONDConn returns a new connection to the SCION daemon of the specified ISD-AS,
// using the SCION environment file.
func sciondConn(ia addr.IA) (daemon.Connector, error) {
	env, err := loadEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading SCION environement: %v\n", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), initTimeout)
	defer cancel()
	conn, err := findSciond(ctx, env, ia)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func findSciond(ctx context.Context, env env.SCION, ia addr.IA) (daemon.Connector, error) {
	as, ok := env.ASes[ia]
	if !ok {
		return nil, fmt.Errorf("AS %v not found in environment", ia)
	}
	sciondConn, err := daemon.NewService(as.DaemonAddress).Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to AS %s SCIOND at %s: %w", ia, as.DaemonAddress, err)
	}
	return sciondConn, nil
}

func loadEnv() (env.SCION, error) {
	envFile := os.Getenv("SCION_ENV_FILE")
	if envFile == "" {
		envFile = "/etc/scion/environment.json"
	}
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
