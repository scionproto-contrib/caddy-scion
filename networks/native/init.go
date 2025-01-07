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
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/scionproto/scion/pkg/snet"
	"go.uber.org/zap"

	"github.com/scionproto-contrib/caddy-scion/networks/pool"
)

var (
	nativeNetwork = NewNetwork(pool.NewUsagePool[string, *conn]())
)

func init() {
	nativeNetwork.logger.Store(zap.NewNop())
	caddy.RegisterNetwork(SCIONUDP, nativeNetwork.Listen)
	caddyhttp.RegisterNetworkHTTP3(SCIONNetwork, SCIONUDP)
}

func SetLogger(logger *zap.Logger) {
	nativeNetwork.SetLogger(logger)
}

func SetPacketConnMetrics(metrics snet.SCIONPacketConnMetrics) {
	nativeNetwork.SetPacketConnMetrics(metrics)
}
