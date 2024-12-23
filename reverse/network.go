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

package reverse

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/scionproto-contrib/http-proxy/reverse"
)

var (
	globalNetwork = reverse.NewNetwork(NewUsagePool[string, reverse.Reusable]())
)

func init() {
	globalNetwork.SetNopLogger()
	caddy.RegisterNetwork(reverse.SCION, globalNetwork.Listen)      // used for HTTP1.1/2 over QUIC/UDP/SCION
	caddy.RegisterNetwork(reverse.SCIONDummy, globalNetwork.Listen) // used to fake HTTP3 over UDP/SCION (on same port as SCION)
	caddy.RegisterNetwork(reverse.SCION3, globalNetwork.Listen)     // used to fake HTTP1.1/2 over TCP/SCION
	caddy.RegisterNetwork(reverse.SCION3QUIC, globalNetwork.Listen) // used for HTTP3 over UDP/SCION
	caddyhttp.RegisterNetworkHTTP3(reverse.SCION3, reverse.SCION3QUIC)
	caddyhttp.RegisterNetworkHTTP3(reverse.SCION, reverse.SCIONDummy)
}

// Interface guards
var (
	_ caddy.ListenerFunc = (*reverse.Network)(nil).Listen
)
