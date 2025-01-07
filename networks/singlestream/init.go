// Copyright 2024 ETH Zurich
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

package singlestream

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/scionproto/scion/pkg/snet"
	"go.uber.org/zap"

	"github.com/scionproto-contrib/http-proxy/networks/dummy"
	"github.com/scionproto-contrib/http-proxy/networks/singlestream"
	"github.com/scionproto-contrib/http-proxy/networks/utils"

	_ "github.com/scionproto-contrib/caddy-scion/networks/dummy"
	"github.com/scionproto-contrib/caddy-scion/networks/pool"
)

var (
	ssNetwork = singlestream.NewNetwork(pool.NewUsagePool[string, utils.Reusable]())
)

func init() {
	ssNetwork.SetNopLogger()
	caddy.RegisterNetwork(singlestream.SCIONSingleStream, ssNetwork.Listen) // used for HTTP1.1/2 over QUIC/UDP/SCION
	caddyhttp.RegisterNetworkHTTP3(singlestream.SCIONSingleStream, dummy.SCIONDummy)
}

func SetLogger(logger *zap.Logger) {
	ssNetwork.SetLogger(logger)
}

func SetPacketConnMetrics(metrics snet.SCIONPacketConnMetrics) {
	ssNetwork.SetPacketConnMetrics(metrics)
}

// Interface guards
var (
	_ caddy.ListenerFunc = (*singlestream.Network)(nil).Listen
)
