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
	"net"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

var (
	// Interface guards
	_ caddy.Provisioner  = (*SCION)(nil)
	_ caddy.Module       = (*SCION)(nil)
	_ caddy.ListenerFunc = (*Network)(nil).Listen

	_ net.Listener   = (*blockedListener)(nil)
	_ net.PacketConn = (*conn)(nil)
)

func init() {
	globalNetwork.logger.Store(zap.NewNop())

	caddy.RegisterModule(SCION{Network: &globalNetwork})
	caddy.RegisterNetwork("scion", globalNetwork.ListenBlocked)
	caddy.RegisterNetwork("scion+quic", globalNetwork.Listen)
	caddyhttp.RegisterNetworkHTTP3("scion", "scion+quic")
}

// SCION implements a caddy module. Currently, it is used to initialize the
// logger for the global network. In the future, additional configuration can be
// parsed with this component.
type SCION struct {
	Network *Network
}

func (SCION) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "scion",
		New: func() caddy.Module {
			return new(SCION)
		},
	}
}

func (s *SCION) Provision(ctx caddy.Context) error {
	s.Network.SetLogger(ctx.Logger(s))
	return nil
}
