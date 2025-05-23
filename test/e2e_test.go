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

//go:build e2e

package test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/caddyserver/caddy/v2/modules/caddypki"
	"github.com/caddyserver/caddy/v2/modules/caddytls"
	"github.com/netsec-ethz/scion-apps/pkg/shttp3"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/private/app/env"
	"go.uber.org/zap"

	caddyscion "github.com/scionproto-contrib/caddy-scion/forward"
	_ "github.com/scionproto-contrib/caddy-scion/reverse"
	_ "github.com/scionproto-contrib/caddy-scion/reverse/detector"
)

var (
	serverAddr = flag.String("server-addr", "127.0.0.1", "local-address for forward/reverse proxy")
	sciondAddr = flag.String("sciond-address", "127.0.0.1:30255", "address to the scion daemon")

	targetServerResponseBody = []byte("hello from test server")
)

const (
	ipHost    = "localhost"
	scionHost = "scion.local"

	forwardProxyHost = "localhost"
	forwardProxyPort = 1443

	reverseProxyIPHTTPsPort           = 2443
	reverseProxySingleStreamHTTPPort  = 7080
	reverseProxySingleStreamHTTPSPort = 7443
	reverseProxyHTTP3Port             = 8443

	targetServerHost               = "localhost"
	targetServerPort               = 3080
	targetServerResponseStatusCode = http.StatusOK

	emptyPolicy = ""
)

func TestGetTargetViaProxy(t *testing.T) {
	tests := []struct {
		name         string
		proxyUseTLS  bool
		targetUseTLS bool
		targetHost   string
		targetPort   int
	}{
		// XXX We only test HTTPS over IP because the reverse proxy is configured to use
		// HTTP (wihtout TLS) over SCION. So far we cannot disable TLS in more than one port.
		// Regarding proxying via HTTP3 is not supported for the Forward proxy, because it is neither
		// supported by browsers.
		{"HTTPsTargetViaHTTPsProxyOverIP", true, true, ipHost, reverseProxyIPHTTPsPort},
		{"HTTPsTargetViaHTTPsProxyOverSCION", true, true, scionHost, reverseProxySingleStreamHTTPSPort},
		{"HTTPTargetViaHTTPsProxyOverSCION", true, false, scionHost, reverseProxySingleStreamHTTPPort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := getViaProxy(forwardProxyHost, forwardProxyPort, tt.proxyUseTLS, tt.targetHost, tt.targetPort, tt.targetUseTLS)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetTargetOverIP(t *testing.T) {
	tests := []struct {
		name   string
		useTLS bool
	}{
		// XXX We only test HTTPS over IP because the reverse proxy is configured to use
		// HTTP (wihtout TLS) over SCION. So far we can only disable TLS in one port.
		{"HTTPsTargetOverIP", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testGetTargetOverIP(t, tt.useTLS)
		})
	}
}

func TestGetTargetOverH3SCION(t *testing.T) {
	t.Run("HTTP3TargetOverSCION", func(t *testing.T) {
		testGetTargetOverH3SCION(t)
	})
}

func TestMain(m *testing.M) {
	flag.Parse()
	// Check for forward proxy
	err := checkScionConfiguration(*sciondAddr)
	if err != nil {
		panic(err)
	}
	// Check for reverse proxy
	ia, err := iaFromEnvironment()
	if err != nil {
		panic(err)
	}

	reverseIPHTTPSAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", *serverAddr, reverseProxyIPHTTPsPort))
	if err != nil {
		panic(err)
	}

	reverseSingleStreamHTTPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *serverAddr, reverseProxySingleStreamHTTPPort))
	if err != nil {
		panic(err)
	}
	reverseSCIONHTTPAddr := &snet.UDPAddr{
		IA:   ia,
		Host: reverseSingleStreamHTTPAddr,
	}

	reverseSingleStreamHTTPSAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *serverAddr, reverseProxySingleStreamHTTPSPort))
	if err != nil {
		panic(err)
	}
	reverseSCIONHTTPSAddr := &snet.UDPAddr{
		IA:   ia,
		Host: reverseSingleStreamHTTPSAddr,
	}

	reverseHTTP3Addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *serverAddr, reverseProxyHTTP3Port))
	if err != nil {
		panic(err)
	}
	reverseSCIONHTTP3Addr := &snet.UDPAddr{
		IA:   ia,
		Host: reverseHTTP3Addr,
	}

	handlerJSON := func(h caddyhttp.MiddlewareHandler) json.RawMessage {
		return caddyconfig.JSONModuleObject(h, "handler", h.(caddy.Module).CaddyModule().ID.Name(), nil)
	}

	hostJSON, err := json.Marshal([]string{ipHost, scionHost})
	if err != nil {
		panic(err)
	}

	httpApp := caddyhttp.App{
		// XXX: This is the only way to disable TLS for the listeners by config.
		// We set here the SingleStream SCION port to test the reverse proxy over SCION without TLS.
		HTTPPort:  reverseProxySingleStreamHTTPPort,
		HTTPSPort: reverseProxySingleStreamHTTPSPort,
		Servers: map[string]*caddyhttp.Server{
			"forward": {
				Listen: []string{fmt.Sprintf(":%d", forwardProxyPort)},
				Routes: caddyhttp.RouteList{
					caddyhttp.Route{
						HandlersRaw: []json.RawMessage{handlerJSON(&caddyscion.Handler{})},
					},
				},
				TLSConnPolicies: []*caddytls.ConnectionPolicy{
					{}, // empty connection policy trigger TLS on all listeners (except HTTPPort)
				},
			},
			"reverse": {
				Listen: []string{
					reverseIPHTTPSAddr.String(),
					fmt.Sprintf("scion+single-stream/%s", reverseSCIONHTTPAddr.String()),
					fmt.Sprintf("scion+single-stream/%s", reverseSCIONHTTPSAddr.String()),
					fmt.Sprintf("scion/%s", reverseSCIONHTTP3Addr.String()),
				},
				Routes: caddyhttp.RouteList{
					caddyhttp.Route{
						MatcherSetsRaw: caddyhttp.RawMatcherSets{
							caddy.ModuleMap{"host": json.RawMessage(hostJSON)},
						},
						HandlersRaw: []json.RawMessage{
							json.RawMessage(`{"handler": "detect_scion"}`),
							handlerJSON(&reverseproxy.Handler{
								Upstreams: reverseproxy.UpstreamPool{
									&reverseproxy.Upstream{
										Dial: fmt.Sprintf("%s:%d", targetServerHost, targetServerPort),
									},
								},
							}),
						},
					},
				},
				ListenProtocols: [][]string{
					{"h1", "h2", "h3"},
					{"h1", "h2"},
					{"h1", "h2"},
					{"h3"},
				},
			},
			"dummy": {
				Listen: []string{fmt.Sprintf(":%d", targetServerPort)},
				Routes: caddyhttp.RouteList{
					caddyhttp.Route{
						HandlersRaw: []json.RawMessage{handlerJSON(&caddyhttp.StaticResponse{
							StatusCode: caddyhttp.WeakString(fmt.Sprintf("%d", targetServerResponseStatusCode)),
							Body:       string(targetServerResponseBody),
						})},
					},
				},
			},
		},
		GracePeriod: caddy.Duration(1 * time.Second), // keep tests fast
	}
	httpAppJSON, err := json.Marshal(httpApp)
	if err != nil {
		panic(err)
	}

	// ensure we always use internal issuer and not a public CA and issue certificate for forward proxy host
	tlsApp := caddytls.TLS{
		CertificatesRaw: caddy.ModuleMap{"automate": json.RawMessage(fmt.Sprintf(`["%s"]`, forwardProxyHost))},
		Automation: &caddytls.AutomationConfig{
			Policies: []*caddytls.AutomationPolicy{
				{
					IssuersRaw: []json.RawMessage{json.RawMessage(`{"module": "internal"}`)},
				},
			},
		},
	}
	tlsAppJSON, err := json.Marshal(tlsApp)
	if err != nil {
		panic(err)
	}

	// configure the default CA so that we don't try to install trust, just for our tests
	falseBool := false
	pkiApp := caddypki.PKI{
		CAs: map[string]*caddypki.CA{
			"local": {InstallTrust: &falseBool},
		},
	}
	pkiAppJSON, err := json.Marshal(pkiApp)
	if err != nil {
		panic(err)
	}

	// build final config
	cfg := &caddy.Config{
		Admin: &caddy.AdminConfig{
			Disabled: true,
			Config: &caddy.ConfigSettings{
				Persist: &falseBool,
			},
		},
		AppsRaw: caddy.ModuleMap{
			"http": httpAppJSON,
			"tls":  tlsAppJSON,
			"pki":  pkiAppJSON,
		},
		Logging: &caddy.Logging{
			Logs: map[string]*caddy.CustomLog{
				"default": {BaseLog: caddy.BaseLog{Level: zap.DebugLevel.CapitalString()}},
			},
		},
	}

	cfgJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(cfgJSON))

	// start the engines
	err = caddy.Run(cfg)
	if err != nil {
		panic(err)
	}

	// wait server ready for tls dial
	time.Sleep(500 * time.Millisecond)

	retCode := m.Run()

	caddy.Stop() // ignore error on shutdown

	os.Exit(retCode)
}

func testGetTargetOverIP(t *testing.T, useTLS bool) {
	if !useTLS {
		panic("HTTP over IP is not supported")
	}
	scheme := "https"
	port := reverseProxyIPHTTPsPort

	client := &http.Client{
		Transport: &http.Transport{
			// We need to skip verification because the certificate from the Caddy endpoint is self-signed
			// and the Go client will not have the CA to verify it.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("%s://%s:%d", scheme, ipHost, port)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get target over IP: %v", err)
	}
	defer resp.Body.Close()

	if err := responseExpected(resp, targetServerResponseStatusCode, targetServerResponseBody); err != nil {
		t.Fatalf("Unexpected response: %v", err)
	}
}

func testGetTargetOverH3SCION(t *testing.T) {
	roundTripper := shttp3.DefaultTransport
	roundTripper.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	defer roundTripper.Close()

	client := &http.Client{
		Transport: roundTripper,
	}

	url := fmt.Sprintf("%s://%s:%d", "https", scionHost, reverseProxyHTTP3Port)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get target over H3/SCION: %v", err)
	}
	defer resp.Body.Close()

	if err := responseExpected(resp, targetServerResponseStatusCode, targetServerResponseBody); err != nil {
		t.Fatalf("Unexpected response: %v", err)
	}
}

func getViaProxy(proxyHost string, proxyPort int, proxyUseTLS bool, targetHost string, targetPort int, targetUseTLS bool) error {
	proxyScheme := "http"
	if proxyUseTLS {
		proxyScheme = "https"
	}

	targetScheme := "http"
	if targetUseTLS {
		targetScheme = "https"
	}

	proxyURL := &url.URL{
		Scheme: proxyScheme,
		Host:   fmt.Sprintf("%s:%d", proxyHost, proxyPort),
		User:   url.UserPassword("policy", emptyPolicy),
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		// We need to skip verification because the certificate from the Caddy forward proxy is self-signed
		// and the Go client will not have the CA to verify it.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		// the go http client does not support http2 proxy, only http1 proxies
		// see: https://github.com/golang/go/issues/26479
		// we would need to create both connection and do the TLS handshake manually
		ForceAttemptHTTP2: false,
	}

	client := &http.Client{Transport: transport}
	url := fmt.Sprintf("%s://%s:%d", targetScheme, targetHost, targetPort)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get via proxy: %w", err)
	}
	defer resp.Body.Close()

	if err := responseExpected(resp, targetServerResponseStatusCode, targetServerResponseBody); err != nil {
		return fmt.Errorf("unexpected response: %w", err)
	}

	return nil
}

func responseExpected(resp *http.Response, expectedStatusCode int, expectedResponse []byte) error {
	if expectedStatusCode != resp.StatusCode {
		return fmt.Errorf("returned wrong status code: got %d want %d",
			resp.StatusCode, targetServerResponseStatusCode)
	}

	responseLen := len(expectedResponse) + 2 // 2 extra bytes is enough to detected that expectedResponse is longer
	response := make([]byte, responseLen)
	var nTotal int
	for {
		n, err := resp.Body.Read(response[nTotal:])
		nTotal += n
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		if nTotal == responseLen {
			return fmt.Errorf("returned nTotal == responseLen, but haven't seen io.EOF:\ngot: '%s'\nwant: '%s'",
				response, expectedResponse)
		}
	}
	response = response[:nTotal]
	if len(expectedResponse) != len(response) {
		return fmt.Errorf("returned wrong response length:\ngot %d: '%s'\nwant %d: '%s'\n",
			len(response), response, len(expectedResponse), expectedResponse)
	}
	for i := range response {
		if response[i] != expectedResponse[i] {
			return fmt.Errorf("returned response has mismatch at character #%d\ngot: '%s'\nwant: '%s'",
				i, response, expectedResponse)
		}
	}
	return nil
}

func checkScionConfiguration(daemonAddr string) error {
	if daemonAddr != "" {
		os.Setenv("SCION_DAEMON_ADDRESS", daemonAddr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sciond, err := findSciond(ctx)
	if err != nil {
		return err
	}

	ia, err := sciond.LocalIA(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Client SCIOND found at %s, local IA: %s\n", daemonAddr, ia)

	return nil
}

func findSciond(ctx context.Context) (daemon.Connector, error) {
	address, ok := os.LookupEnv("SCION_DAEMON_ADDRESS")
	if !ok {
		address = daemon.DefaultAPIAddress
	}

	sciondConn, err := daemon.NewService(address).Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to SCIOND at %s (provide as flag or override with SCION_DAEMON_ADDRESS): %w", address, err)
	}
	return sciondConn, nil
}

func iaFromEnvironment() (addr.IA, error) {
	e, err := loadEnv()
	if err != nil {
		return addr.IA(0), fmt.Errorf("error loading SCION environment: %w", err)
	}
	if len(e.ASes) != 1 {
		return addr.IA(0), fmt.Errorf("expected exactly one AS in the environment, got %d", len(e.ASes))
	}
	var ia addr.IA
	var as env.AS
	for k, a := range e.ASes {
		ia = k
		as = a
		break
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := checkSciond(ctx, as); err != nil {
		return addr.IA(0), fmt.Errorf("unable to connect to AS %s SCIOND at %s: %w", ia, as.DaemonAddress, err)
	}
	fmt.Printf("Server SCIOND found at %s, local IA: %s\n", as.DaemonAddress, ia)
	return ia, nil
}

func checkSciond(ctx context.Context, as env.AS) error {
	_, err := daemon.NewService(as.DaemonAddress).Connect(ctx)
	if err != nil {
		return err
	}
	os.Setenv("SCION_DAEMON_ADDRESS", as.DaemonAddress)
	return nil
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
