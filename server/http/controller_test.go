// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/server/registry"
)

func TestGetConfFromConfigurerKeepsPluginFields(t *testing.T) {
	cfg := &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: "test-proxy",
			Type: string(v1.ProxyTypeTCP),
			ProxyBackend: v1.ProxyBackend{
				Plugin: v1.TypedClientPluginOptions{
					Type: v1.PluginHTTPProxy,
					ClientPluginOptions: &v1.HTTPProxyPluginOptions{
						Type:         v1.PluginHTTPProxy,
						HTTPUser:     "user",
						HTTPPassword: "password",
					},
				},
			},
		},
		RemotePort: 6000,
	}

	content, err := json.Marshal(getConfFromConfigurer(cfg))
	if err != nil {
		t.Fatalf("marshal conf failed: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(content, &out); err != nil {
		t.Fatalf("unmarshal conf failed: %v", err)
	}

	pluginValue, ok := out["plugin"]
	if !ok {
		t.Fatalf("plugin field missing in output: %v", out)
	}
	plugin, ok := pluginValue.(map[string]any)
	if !ok {
		t.Fatalf("plugin field should be object, got: %#v", pluginValue)
	}

	if got := plugin["type"]; got != v1.PluginHTTPProxy {
		t.Fatalf("plugin type mismatch, want %q got %#v", v1.PluginHTTPProxy, got)
	}
	if got := plugin["httpUser"]; got != "user" {
		t.Fatalf("plugin httpUser mismatch, want %q got %#v", "user", got)
	}
	if got := plugin["httpPassword"]; got != "password" {
		t.Fatalf("plugin httpPassword mismatch, want %q got %#v", "password", got)
	}
}

type stubGatewayTunnelManager struct {
	tunnels       map[string]gatewaypkg.Tunnel
	syncCalls     []string
	refreshCalls  [][]string
	createErr     error
	updateErr     error
	deleteErr     error
	nextGenerated int
}

func newStubGatewayTunnelManager() *stubGatewayTunnelManager {
	return &stubGatewayTunnelManager{
		tunnels:       make(map[string]gatewaypkg.Tunnel),
		nextGenerated: 1,
	}
}

func (m *stubGatewayTunnelManager) List() []gatewaypkg.Tunnel {
	items := make([]gatewaypkg.Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		items = append(items, tunnel)
	}
	return items
}

func (m *stubGatewayTunnelManager) Get(id string) (gatewaypkg.Tunnel, bool) {
	tunnel, ok := m.tunnels[id]
	return tunnel, ok
}

func (m *stubGatewayTunnelManager) Create(tunnel gatewaypkg.Tunnel) (gatewaypkg.Tunnel, error) {
	if m.createErr != nil {
		return gatewaypkg.Tunnel{}, m.createErr
	}
	if tunnel.ID == "" {
		tunnel.ID = "generated-id"
		if m.nextGenerated > 1 {
			tunnel.ID += string(rune('0' + m.nextGenerated - 1))
		}
		m.nextGenerated++
	}
	m.tunnels[tunnel.ID] = tunnel
	return tunnel, nil
}

func (m *stubGatewayTunnelManager) Update(id string, tunnel gatewaypkg.Tunnel) (gatewaypkg.Tunnel, error) {
	if m.updateErr != nil {
		return gatewaypkg.Tunnel{}, m.updateErr
	}
	tunnel.ID = id
	m.tunnels[id] = tunnel
	return tunnel, nil
}

func (m *stubGatewayTunnelManager) Delete(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.tunnels, id)
	return nil
}

func (m *stubGatewayTunnelManager) RefreshStatus(_ context.Context, tunnelIDs []string) {
	copied := append([]string(nil), tunnelIDs...)
	m.refreshCalls = append(m.refreshCalls, copied)
}

func (m *stubGatewayTunnelManager) SyncClient(clientKey string) error {
	m.syncCalls = append(m.syncCalls, clientKey)
	return nil
}

func newGatewayContext(t *testing.T, method, target string, body []byte, vars map[string]string) *httppkg.Context {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	rec := httptest.NewRecorder()
	return httppkg.NewContext(rec, req)
}

func registerClient(
	t *testing.T,
	reg *registry.ClientRegistry,
	rawClientID string,
	runID string,
	allowGateway bool,
) string {
	t.Helper()

	key, conflict := reg.Register(
		"user",
		rawClientID,
		runID,
		"host",
		"0.68.1-mix.7",
		"127.0.0.1",
		"tcp",
		allowGateway,
	)
	require.False(t, conflict)
	return key
}

func TestAPICreateGatewayTunnelRejectsClientWithoutStableClientID(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "", "run-no-client-id", true)
	manager := newStubGatewayTunnelManager()
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	body := []byte(`{"name":"ssh","protocol":"tcp","bindAddr":"0.0.0.0","listenPort":6000,"clientKey":"` + key + `","targetHost":"127.0.0.1","targetPort":22}`)
	_, err := controller.APICreateGatewayTunnel(newGatewayContext(t, "POST", "/api/gateway-tunnels", body, nil))
	require.Error(t, err)

	httpErr, ok := err.(*httppkg.Error)
	require.True(t, ok)
	require.Equal(t, "selected gateway client must configure clientID", httpErr.Error())
}

func TestAPICreateGatewayTunnelRejectsClientWithoutGatewayOptIn(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-disabled", "run-disabled", false)
	manager := newStubGatewayTunnelManager()
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	body := []byte(`{"name":"ssh","protocol":"tcp","bindAddr":"0.0.0.0","listenPort":6000,"clientKey":"` + key + `","targetHost":"127.0.0.1","targetPort":22}`)
	_, err := controller.APICreateGatewayTunnel(newGatewayContext(t, "POST", "/api/gateway-tunnels", body, nil))
	require.Error(t, err)

	httpErr, ok := err.(*httppkg.Error)
	require.True(t, ok)
	require.Equal(t, "selected gateway client does not allow gateway tunnels", httpErr.Error())
}

func TestAPICreateGatewayTunnelCreatesAndSyncs(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-a", "run-a", true)
	manager := newStubGatewayTunnelManager()
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	body := []byte(`{"name":"ssh","remark":"ops","protocol":"tcp","bindAddr":"127.0.0.1","listenPort":6000,"clientKey":"` + key + `","targetHost":"127.0.0.1","targetPort":22}`)
	resp, err := controller.APICreateGatewayTunnel(newGatewayContext(t, "POST", "/api/gateway-tunnels", body, nil))
	require.NoError(t, err)

	tunnel, ok := resp.(gatewaypkg.Tunnel)
	require.True(t, ok)
	require.Equal(t, "generated-id", tunnel.ID)
	require.Equal(t, key, tunnel.ClientKey)
	require.Equal(t, []string{key}, manager.syncCalls)
	require.Len(t, manager.refreshCalls, 1)
	require.Equal(t, []string{"generated-id"}, manager.refreshCalls[0])
}

func TestAPIUpdateGatewayTunnelSyncsPreviousAndCurrentClients(t *testing.T) {
	reg := registry.NewClientRegistry()
	oldKey := registerClient(t, reg, "client-old", "run-old", true)
	newKey := registerClient(t, reg, "client-new", "run-new", true)
	manager := newStubGatewayTunnelManager()
	manager.tunnels["t-1"] = gatewaypkg.Tunnel{
		ID:         "t-1",
		Name:       "ssh",
		Protocol:   "tcp",
		BindAddr:   "0.0.0.0",
		ListenPort: 6000,
		ClientKey:  oldKey,
		TargetHost: "127.0.0.1",
		TargetPort: 22,
	}
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	body := []byte(`{"name":"ssh","remark":"new","protocol":"udp","bindAddr":"127.0.0.1","listenPort":7000,"clientKey":"` + newKey + `","targetHost":"127.0.0.1","targetPort":53}`)
	resp, err := controller.APIUpdateGatewayTunnel(newGatewayContext(t, "PUT", "/api/gateway-tunnels/t-1", body, map[string]string{"id": "t-1"}))
	require.NoError(t, err)

	tunnel, ok := resp.(gatewaypkg.Tunnel)
	require.True(t, ok)
	require.Equal(t, "t-1", tunnel.ID)
	require.Equal(t, newKey, tunnel.ClientKey)
	require.Equal(t, []string{oldKey, newKey}, manager.syncCalls)
	require.Len(t, manager.refreshCalls, 1)
	require.Equal(t, []string{"t-1"}, manager.refreshCalls[0])
}

func TestAPIDeleteGatewayTunnelSyncsClient(t *testing.T) {
	manager := newStubGatewayTunnelManager()
	manager.tunnels["t-1"] = gatewaypkg.Tunnel{
		ID:        "t-1",
		Name:      "ssh",
		ClientKey: "client-a",
	}
	controller := NewController(&v1.ServerConfig{}, nil, nil, manager)

	resp, err := controller.APIDeleteGatewayTunnel(newGatewayContext(t, "DELETE", "/api/gateway-tunnels/t-1", nil, map[string]string{"id": "t-1"}))
	require.NoError(t, err)
	require.Equal(t, httppkg.GeneralResponse{Code: 200, Msg: "success"}, resp)
	require.Equal(t, []string{"client-a"}, manager.syncCalls)
	_, ok := manager.tunnels["t-1"]
	require.False(t, ok)
}

func TestAPICreateGatewayTunnelSurfacesManagerValidationError(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-a", "run-a", true)
	manager := newStubGatewayTunnelManager()
	manager.createErr = errors.New("listenPort must be between 1 and 65535")
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	body := []byte(`{"name":"ssh","protocol":"tcp","bindAddr":"0.0.0.0","listenPort":0,"clientKey":"` + key + `","targetHost":"127.0.0.1","targetPort":22}`)
	_, err := controller.APICreateGatewayTunnel(newGatewayContext(t, "POST", "/api/gateway-tunnels", body, nil))
	require.Error(t, err)

	httpErr, ok := err.(*httppkg.Error)
	require.True(t, ok)
	require.Equal(t, "listenPort must be between 1 and 65535", httpErr.Error())
}

func TestAPIGatewayTunnelExportYAML(t *testing.T) {
	manager := newStubGatewayTunnelManager()
	manager.tunnels["t-1"] = gatewaypkg.Tunnel{
		ID:         "t-1",
		Name:       "ssh-main",
		Remark:     "ops",
		Protocol:   "tcp",
		BindAddr:   "0.0.0.0",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 22,
		Status:     gatewaypkg.StatusOnline,
	}
	controller := NewController(&v1.ServerConfig{}, nil, nil, manager)

	resp, err := controller.APIGatewayTunnelExport(newGatewayContext(t, "GET", "/api/gateway-tunnels/export", nil, nil))
	require.NoError(t, err)

	respMap, ok := resp.(map[string]any)
	require.True(t, ok)
	rawYAML, ok := respMap["yaml"].(string)
	require.True(t, ok)
	require.NotEmpty(t, strings.TrimSpace(rawYAML))

	parsed, err := parseGatewayTunnelYAML(rawYAML)
	require.NoError(t, err)
	require.Equal(t, 1, parsed.Version)
	require.Equal(t, 1, parsed.TunnelCount)
	require.Len(t, parsed.Tunnels, 1)
	require.Equal(t, "ssh-main", parsed.Tunnels[0].Name)
	require.Equal(t, "client-a", parsed.Tunnels[0].ClientKey)
}

func TestAPIGatewayTunnelImportUpsert(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-a", "run-a", true)

	manager := newStubGatewayTunnelManager()
	manager.tunnels["existing-id"] = gatewaypkg.Tunnel{
		ID:         "existing-id",
		Name:       "ssh-main",
		Protocol:   "tcp",
		BindAddr:   "0.0.0.0",
		ListenPort: 6000,
		ClientKey:  key,
		TargetHost: "127.0.0.1",
		TargetPort: 22,
	}
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	rawYAML := `version: 1
tunnels:
  - name: ssh-main
    remark: changed
    protocol: tcp
    bindAddr: 127.0.0.1
    listenPort: 6000
    clientKey: ` + key + `
    targetHost: 127.0.0.1
    targetPort: 2222
  - name: dns-new
    protocol: udp
    bindAddr: 0.0.0.0
    listenPort: 5300
    clientKey: ` + key + `
    targetHost: 127.0.0.1
    targetPort: 53
`
	body, err := json.Marshal(map[string]string{"yaml": rawYAML})
	require.NoError(t, err)

	resp, err := controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.NoError(t, err)

	result, ok := resp.(gatewayTunnelImportResponse)
	require.True(t, ok)
	require.Equal(t, 2, result.Total)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Updated)

	require.Len(t, manager.tunnels, 2)
	updated := manager.tunnels["existing-id"]
	require.Equal(t, 2222, updated.TargetPort)
	require.Equal(t, "changed", updated.Remark)
	require.Equal(t, "127.0.0.1", updated.BindAddr)
	require.Equal(t, []string{key}, manager.syncCalls)
	require.Len(t, manager.refreshCalls, 1)
	require.ElementsMatch(t, []string{"existing-id", "generated-id"}, manager.refreshCalls[0])
}

func TestAPIGatewayTunnelImportRejectsInvalidYAML(t *testing.T) {
	controller := NewController(&v1.ServerConfig{}, nil, nil, newStubGatewayTunnelManager())

	body, err := json.Marshal(map[string]string{"yaml": "not: [valid"})
	require.NoError(t, err)

	_, err = controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.Error(t, err)
	httpErr, ok := err.(*httppkg.Error)
	require.True(t, ok)
	require.Contains(t, httpErr.Error(), "parse YAML error")
}

func TestAPIGatewayTunnelImportSupportsListYAML(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-a", "run-a", true)
	controller := NewController(&v1.ServerConfig{}, reg, nil, newStubGatewayTunnelManager())

	rawYAML := `- name: ssh-main
  protocol: tcp
  bindAddr: 0.0.0.0
  listenPort: 6000
  clientKey: ` + key + `
  targetHost: 127.0.0.1
  targetPort: 22
`
	body, err := json.Marshal(map[string]string{"yaml": rawYAML})
	require.NoError(t, err)

	resp, err := controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.NoError(t, err)
	result, ok := resp.(gatewayTunnelImportResponse)
	require.True(t, ok)
	require.Equal(t, 1, result.Total)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Updated)
}

func TestAPIGatewayTunnelImportRejectsEmptyList(t *testing.T) {
	controller := NewController(&v1.ServerConfig{}, nil, nil, newStubGatewayTunnelManager())
	body, err := json.Marshal(map[string]string{"yaml": "tunnels: []"})
	require.NoError(t, err)

	_, err = controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.Error(t, err)
	httpErr, ok := err.(*httppkg.Error)
	require.True(t, ok)
	require.Equal(t, "no gateway tunnels found in yaml", httpErr.Error())
}

func TestAPIGatewayTunnelImportIsIdempotentOnRepeatedImport(t *testing.T) {
	reg := registry.NewClientRegistry()
	key := registerClient(t, reg, "client-a", "run-a", true)
	manager := newStubGatewayTunnelManager()
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	rawYAML := `version: 1
tunnels:
  - name: ssh-main
    protocol: tcp
    bindAddr: 0.0.0.0
    listenPort: 6000
    clientKey: ` + key + `
    targetHost: 127.0.0.1
    targetPort: 22
`
	body, err := json.Marshal(map[string]string{"yaml": rawYAML})
	require.NoError(t, err)

	firstResp, err := controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.NoError(t, err)
	firstResult := firstResp.(gatewayTunnelImportResponse)
	require.Equal(t, 1, firstResult.Total)
	require.Equal(t, 1, firstResult.Created)
	require.Equal(t, 0, firstResult.Updated)

	secondResp, err := controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.NoError(t, err)
	secondResult := secondResp.(gatewayTunnelImportResponse)
	require.Equal(t, 1, secondResult.Total)
	require.Equal(t, 0, secondResult.Created)
	require.Equal(t, 1, secondResult.Updated)
}

func TestAPIGatewayTunnelImportAllowsUnknownClientKey(t *testing.T) {
	reg := registry.NewClientRegistry()
	manager := newStubGatewayTunnelManager()
	controller := NewController(&v1.ServerConfig{}, reg, nil, manager)

	rawYAML := `version: 1
tunnels:
  - name: ssh-main
    protocol: tcp
    bindAddr: 0.0.0.0
    listenPort: 6000
    clientKey: user.client-offline
    targetHost: 127.0.0.1
    targetPort: 22
`
	body, err := json.Marshal(map[string]string{"yaml": rawYAML})
	require.NoError(t, err)

	resp, err := controller.APIGatewayTunnelImport(newGatewayContext(t, "POST", "/api/gateway-tunnels/import", body, nil))
	require.NoError(t, err)
	result := resp.(gatewayTunnelImportResponse)
	require.Equal(t, 1, result.Total)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Updated)
}
