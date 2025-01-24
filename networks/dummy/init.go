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

	"github.com/scionproto-contrib/http-proxy/networks"
	"github.com/scionproto-contrib/http-proxy/networks/dummy"

	"github.com/scionproto-contrib/caddy-scion/networks/pool"
)

var (
	dummyNetwork = dummy.NewNetwork(newUsagePoolWrapper[string, networks.Reusable]())
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

type usagePoolWrapper[K comparable, V any] struct {
	usagePool *pool.UsagePool[K, V]
}

func newUsagePoolWrapper[K comparable, V any]() *usagePoolWrapper[K, V] {
	return &usagePoolWrapper[K, V]{
		usagePool: pool.NewUsagePool[K, V](),
	}
}

func (w *usagePoolWrapper[K, V]) LoadOrNew(key K, construct func() (networks.Destructor, error)) (V, bool, error) {
	return w.usagePool.LoadOrNew(key, func() (caddy.Destructor, error) {
		return construct()
	})
}

func (w *usagePoolWrapper[K, V]) Delete(key K) (bool, error) {
	return w.usagePool.Delete(key)
}
