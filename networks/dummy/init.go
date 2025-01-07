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

package dummy

import (
	"github.com/caddyserver/caddy/v2"
	"go.uber.org/zap"

	"github.com/scionproto-contrib/http-proxy/networks/dummy"
	"github.com/scionproto-contrib/http-proxy/networks/utils"

	"github.com/scionproto-contrib/caddy-scion/networks/pool"
)

var (
	dummyNetwork = dummy.NewNetwork(pool.NewUsagePool[string, utils.Reusable]())
)

func init() {
	dummyNetwork.SetNopLogger()
	caddy.RegisterNetwork("scion", dummyNetwork.Listen)          // used to fake HTTP on a SCION port
	caddy.RegisterNetwork(dummy.SCIONDummy, dummyNetwork.Listen) // used to fake HTTP on a SCION port
}

// Interface guards
var (
	_ caddy.ListenerFunc = (*dummy.Network)(nil).Listen
)

func SetLogger(logger *zap.Logger) {
	dummyNetwork.SetLogger(logger)
}
