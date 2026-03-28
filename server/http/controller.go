// Copyright 2025 The frp Authors
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
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fatedier/frp/pkg/config/types"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/metrics/mem"
	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/pkg/util/jsonx"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/version"
	"github.com/fatedier/frp/server/http/model"
	"github.com/fatedier/frp/server/proxy"
	"github.com/fatedier/frp/server/registry"
)

type Controller struct {
	// dependencies
	serverCfg      *v1.ServerConfig
	clientRegistry *registry.ClientRegistry
	pxyManager     ProxyManager
	gatewayManager GatewayTunnelManager
}

type ProxyManager interface {
	GetByName(name string) (proxy.Proxy, bool)
}

type GatewayTunnelManager interface {
	List() []gatewaypkg.Tunnel
	Get(id string) (gatewaypkg.Tunnel, bool)
	Create(tunnel gatewaypkg.Tunnel) (gatewaypkg.Tunnel, error)
	Update(id string, tunnel gatewaypkg.Tunnel) (gatewaypkg.Tunnel, error)
	Delete(id string) error
	RefreshStatus(ctx context.Context, tunnelIDs []string)
	SyncClient(clientKey string) error
}

type gatewayTunnelYAML struct {
	Name       string `json:"name" yaml:"name"`
	Remark     string `json:"remark,omitempty" yaml:"remark,omitempty"`
	Protocol   string `json:"protocol" yaml:"protocol"`
	BindAddr   string `json:"bindAddr" yaml:"bindAddr"`
	ListenPort int    `json:"listenPort" yaml:"listenPort"`
	ClientKey  string `json:"clientKey" yaml:"clientKey"`
	TargetHost string `json:"targetHost" yaml:"targetHost"`
	TargetPort int    `json:"targetPort" yaml:"targetPort"`
}

type gatewayTunnelExportResponse struct {
	Version     int                 `json:"version" yaml:"version"`
	ExportedAt  int64               `json:"exportedAt" yaml:"exportedAt"`
	TunnelCount int                 `json:"tunnelCount" yaml:"tunnelCount"`
	Tunnels     []gatewayTunnelYAML `json:"tunnels" yaml:"tunnels"`
}

type gatewayTunnelImportRequest struct {
	YAML string `json:"yaml"`
}

type gatewayTunnelImportResponse struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
}

func NewController(
	serverCfg *v1.ServerConfig,
	clientRegistry *registry.ClientRegistry,
	pxyManager ProxyManager,
	gatewayManager GatewayTunnelManager,
) *Controller {
	return &Controller{
		serverCfg:      serverCfg,
		clientRegistry: clientRegistry,
		pxyManager:     pxyManager,
		gatewayManager: gatewayManager,
	}
}

// /api/serverinfo
func (c *Controller) APIServerInfo(ctx *httppkg.Context) (any, error) {
	serverStats := mem.StatsCollector.GetServer()
	svrResp := model.ServerInfoResp{
		Version:               version.Full(),
		BindPort:              c.serverCfg.BindPort,
		VhostHTTPPort:         c.serverCfg.VhostHTTPPort,
		VhostHTTPSPort:        c.serverCfg.VhostHTTPSPort,
		TCPMuxHTTPConnectPort: c.serverCfg.TCPMuxHTTPConnectPort,
		KCPBindPort:           c.serverCfg.KCPBindPort,
		QUICBindPort:          c.serverCfg.QUICBindPort,
		SubdomainHost:         c.serverCfg.SubDomainHost,
		MaxPoolCount:          c.serverCfg.Transport.MaxPoolCount,
		MaxPortsPerClient:     c.serverCfg.MaxPortsPerClient,
		HeartBeatTimeout:      c.serverCfg.Transport.HeartbeatTimeout,
		AllowPortsStr:         types.PortsRangeSlice(c.serverCfg.AllowPorts).String(),
		TLSForce:              c.serverCfg.Transport.TLS.Force,

		TotalTrafficIn:  serverStats.TotalTrafficIn,
		TotalTrafficOut: serverStats.TotalTrafficOut,
		CurConns:        serverStats.CurConns,
		ClientCounts:    serverStats.ClientCounts,
		ProxyTypeCounts: serverStats.ProxyTypeCounts,
	}

	return svrResp, nil
}

// /api/clients
func (c *Controller) APIClientList(ctx *httppkg.Context) (any, error) {
	if c.clientRegistry == nil {
		return nil, fmt.Errorf("client registry unavailable")
	}

	userFilter := ctx.Query("user")
	clientIDFilter := ctx.Query("clientId")
	runIDFilter := ctx.Query("runId")
	statusFilter := strings.ToLower(ctx.Query("status"))

	records := c.clientRegistry.List()
	items := make([]model.ClientInfoResp, 0, len(records))
	for _, info := range records {
		if userFilter != "" && info.User != userFilter {
			continue
		}
		if clientIDFilter != "" && info.ClientID() != clientIDFilter {
			continue
		}
		if runIDFilter != "" && info.RunID != runIDFilter {
			continue
		}
		if !matchStatusFilter(info.Online, statusFilter) {
			continue
		}
		items = append(items, buildClientInfoResp(info))
	}

	slices.SortFunc(items, func(a, b model.ClientInfoResp) int {
		if v := cmp.Compare(a.User, b.User); v != 0 {
			return v
		}
		if v := cmp.Compare(a.ClientID, b.ClientID); v != 0 {
			return v
		}
		return cmp.Compare(a.Key, b.Key)
	})

	return items, nil
}

// /api/clients/{key}
func (c *Controller) APIClientDetail(ctx *httppkg.Context) (any, error) {
	key := ctx.Param("key")
	if key == "" {
		return nil, fmt.Errorf("missing client key")
	}

	if c.clientRegistry == nil {
		return nil, fmt.Errorf("client registry unavailable")
	}

	info, ok := c.clientRegistry.GetByKey(key)
	if !ok {
		return nil, httppkg.NewError(http.StatusNotFound, fmt.Sprintf("client %s not found", key))
	}

	return buildClientInfoResp(info), nil
}

// /api/proxy/:type
func (c *Controller) APIProxyByType(ctx *httppkg.Context) (any, error) {
	proxyType := ctx.Param("type")

	proxyInfoResp := model.GetProxyInfoResp{}
	proxyInfoResp.Proxies = c.getProxyStatsByType(proxyType)
	slices.SortFunc(proxyInfoResp.Proxies, func(a, b *model.ProxyStatsInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return proxyInfoResp, nil
}

// /api/proxy/:type/:name
func (c *Controller) APIProxyByTypeAndName(ctx *httppkg.Context) (any, error) {
	proxyType := ctx.Param("type")
	name := ctx.Param("name")

	proxyStatsResp, code, msg := c.getProxyStatsByTypeAndName(proxyType, name)
	if code != 200 {
		return nil, httppkg.NewError(code, msg)
	}

	return proxyStatsResp, nil
}

// /api/traffic/:name
func (c *Controller) APIProxyTraffic(ctx *httppkg.Context) (any, error) {
	name := ctx.Param("name")

	trafficResp := model.GetProxyTrafficResp{}
	trafficResp.Name = name
	proxyTrafficInfo := mem.StatsCollector.GetProxyTraffic(name)

	if proxyTrafficInfo == nil {
		return nil, httppkg.NewError(http.StatusNotFound, "no proxy info found")
	}
	trafficResp.TrafficIn = proxyTrafficInfo.TrafficIn
	trafficResp.TrafficOut = proxyTrafficInfo.TrafficOut

	return trafficResp, nil
}

// /api/proxies/:name
func (c *Controller) APIProxyByName(ctx *httppkg.Context) (any, error) {
	name := ctx.Param("name")

	ps := mem.StatsCollector.GetProxyByName(name)
	if ps == nil {
		return nil, httppkg.NewError(http.StatusNotFound, "no proxy info found")
	}

	proxyInfo := model.GetProxyStatsResp{
		Name:            ps.Name,
		User:            ps.User,
		ClientID:        ps.ClientID,
		TodayTrafficIn:  ps.TodayTrafficIn,
		TodayTrafficOut: ps.TodayTrafficOut,
		CurConns:        ps.CurConns,
		LastStartTime:   ps.LastStartTime,
		LastCloseTime:   ps.LastCloseTime,
	}

	if pxy, ok := c.pxyManager.GetByName(name); ok {
		proxyInfo.Conf = getConfFromConfigurer(pxy.GetConfigurer())
		proxyInfo.Status = "online"
	} else {
		proxyInfo.Status = "offline"
	}

	return proxyInfo, nil
}

// DELETE /api/proxies?status=offline
func (c *Controller) DeleteProxies(ctx *httppkg.Context) (any, error) {
	status := ctx.Query("status")
	if status != "offline" {
		return nil, httppkg.NewError(http.StatusBadRequest, "status only support offline")
	}
	cleared, total := mem.StatsCollector.ClearOfflineProxies()
	log.Infof("cleared [%d] offline proxies, total [%d] proxies", cleared, total)
	return httppkg.GeneralResponse{Code: 200, Msg: "success"}, nil
}

func (c *Controller) APIGatewayTunnelList(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}
	if ctx.Query("refresh") != "false" {
		c.gatewayManager.RefreshStatus(ctx.Req.Context(), nil)
	}
	return c.gatewayManager.List(), nil
}

func (c *Controller) APIGatewayTunnelDetail(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return nil, httppkg.NewError(http.StatusBadRequest, "gateway tunnel id is required")
	}
	if ctx.Query("refresh") != "false" {
		c.gatewayManager.RefreshStatus(ctx.Req.Context(), []string{id})
	}
	tunnel, ok := c.gatewayManager.Get(id)
	if !ok {
		return nil, httppkg.NewError(http.StatusNotFound, fmt.Sprintf("gateway tunnel %q not found", id))
	}
	return tunnel, nil
}

func (c *Controller) APICreateGatewayTunnel(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}
	tunnel, err := c.parseGatewayTunnelPayload(ctx)
	if err != nil {
		return nil, err
	}
	created, err := c.gatewayManager.Create(tunnel)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	_ = c.gatewayManager.SyncClient(created.ClientKey)
	c.gatewayManager.RefreshStatus(ctx.Req.Context(), []string{created.ID})
	updated, ok := c.gatewayManager.Get(created.ID)
	if !ok {
		return nil, httppkg.NewError(http.StatusInternalServerError, "gateway tunnel disappeared after creation")
	}
	return updated, nil
}

func (c *Controller) APIUpdateGatewayTunnel(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return nil, httppkg.NewError(http.StatusBadRequest, "gateway tunnel id is required")
	}
	previous, ok := c.gatewayManager.Get(id)
	if !ok {
		return nil, httppkg.NewError(http.StatusNotFound, fmt.Sprintf("gateway tunnel %q not found", id))
	}
	tunnel, err := c.parseGatewayTunnelPayload(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := c.gatewayManager.Update(id, tunnel)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	if previous.ClientKey != updated.ClientKey {
		_ = c.gatewayManager.SyncClient(previous.ClientKey)
	}
	_ = c.gatewayManager.SyncClient(updated.ClientKey)
	c.gatewayManager.RefreshStatus(ctx.Req.Context(), []string{id})
	updated, ok = c.gatewayManager.Get(id)
	if !ok {
		return nil, httppkg.NewError(http.StatusInternalServerError, "gateway tunnel disappeared after update")
	}
	return updated, nil
}

func (c *Controller) APIDeleteGatewayTunnel(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}
	id := strings.TrimSpace(ctx.Param("id"))
	if id == "" {
		return nil, httppkg.NewError(http.StatusBadRequest, "gateway tunnel id is required")
	}
	tunnel, ok := c.gatewayManager.Get(id)
	if !ok {
		return nil, httppkg.NewError(http.StatusNotFound, fmt.Sprintf("gateway tunnel %q not found", id))
	}
	if err := c.gatewayManager.Delete(id); err != nil {
		return nil, httppkg.NewError(http.StatusInternalServerError, err.Error())
	}
	_ = c.gatewayManager.SyncClient(tunnel.ClientKey)
	return httppkg.GeneralResponse{Code: 200, Msg: "success"}, nil
}

func (c *Controller) APIGatewayTunnelExport(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}

	if ctx.Query("refresh") == "true" {
		c.gatewayManager.RefreshStatus(ctx.Req.Context(), nil)
	}
	items := c.gatewayManager.List()
	exportItems := make([]gatewayTunnelYAML, 0, len(items))
	for _, item := range items {
		exportItems = append(exportItems, gatewayTunnelYAML{
			Name:       item.Name,
			Remark:     item.Remark,
			Protocol:   item.Protocol,
			BindAddr:   item.BindAddr,
			ListenPort: item.ListenPort,
			ClientKey:  item.ClientKey,
			TargetHost: item.TargetHost,
			TargetPort: item.TargetPort,
		})
	}

	payload := gatewayTunnelExportResponse{
		Version:     1,
		ExportedAt:  time.Now().Unix(),
		TunnelCount: len(exportItems),
		Tunnels:     exportItems,
	}
	content, err := yaml.Marshal(payload)
	if err != nil {
		return nil, httppkg.NewError(http.StatusInternalServerError, fmt.Sprintf("marshal gateway tunnel yaml error: %v", err))
	}
	return map[string]any{
		"yaml": string(content),
	}, nil
}

func (c *Controller) APIGatewayTunnelImport(ctx *httppkg.Context) (any, error) {
	if c.gatewayManager == nil {
		return nil, httppkg.NewError(http.StatusNotImplemented, "gateway tunnels are unavailable")
	}

	body, err := ctx.Body()
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("read body error: %v", err))
	}
	req := gatewayTunnelImportRequest{}
	if err := jsonx.Unmarshal(body, &req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("parse JSON error: %v", err))
	}
	rawYAML := strings.TrimSpace(req.YAML)
	if rawYAML == "" {
		return nil, httppkg.NewError(http.StatusBadRequest, "yaml is required")
	}

	payload, err := parseGatewayTunnelYAML(rawYAML)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	if len(payload.Tunnels) == 0 {
		return nil, httppkg.NewError(http.StatusBadRequest, "no gateway tunnels found in yaml")
	}

	existing := c.gatewayManager.List()
	existingIndex := make(map[string]gatewaypkg.Tunnel, len(existing))
	for _, item := range existing {
		existingIndex[gatewayTunnelIdentity(item.ClientKey, item.Name)] = item
	}

	changedIDs := make([]string, 0, len(payload.Tunnels))
	syncClientKeys := make(map[string]struct{})
	createdCount := 0
	updatedCount := 0

	for idx, item := range payload.Tunnels {
		tunnel := gatewaypkg.Tunnel{
			Name:       strings.TrimSpace(item.Name),
			Remark:     strings.TrimSpace(item.Remark),
			Protocol:   strings.TrimSpace(item.Protocol),
			BindAddr:   strings.TrimSpace(item.BindAddr),
			ListenPort: item.ListenPort,
			ClientKey:  strings.TrimSpace(item.ClientKey),
			TargetHost: strings.TrimSpace(item.TargetHost),
			TargetPort: item.TargetPort,
		}
		if err := c.validateGatewayTunnelImportPayload(tunnel); err != nil {
			return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("invalid tunnel at index %d: %v", idx, err))
		}

		key := gatewayTunnelIdentity(tunnel.ClientKey, tunnel.Name)
		existingTunnel, ok := existingIndex[key]
		if ok {
			updated, err := c.gatewayManager.Update(existingTunnel.ID, tunnel)
			if err != nil {
				return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("update tunnel %q for client %q failed: %v", tunnel.Name, tunnel.ClientKey, err))
			}
			existingIndex[key] = updated
			syncClientKeys[updated.ClientKey] = struct{}{}
			changedIDs = append(changedIDs, updated.ID)
			updatedCount++
			continue
		}

		created, err := c.gatewayManager.Create(tunnel)
		if err != nil {
			return nil, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("create tunnel %q for client %q failed: %v", tunnel.Name, tunnel.ClientKey, err))
		}
		existingIndex[key] = created
		syncClientKeys[created.ClientKey] = struct{}{}
		changedIDs = append(changedIDs, created.ID)
		createdCount++
	}

	for clientKey := range syncClientKeys {
		_ = c.gatewayManager.SyncClient(clientKey)
	}
	if len(changedIDs) > 0 {
		c.gatewayManager.RefreshStatus(ctx.Req.Context(), changedIDs)
	}

	return gatewayTunnelImportResponse{
		Total:   len(payload.Tunnels),
		Created: createdCount,
		Updated: updatedCount,
	}, nil
}

func (c *Controller) parseGatewayTunnelPayload(ctx *httppkg.Context) (gatewaypkg.Tunnel, error) {
	body, err := ctx.Body()
	if err != nil {
		return gatewaypkg.Tunnel{}, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("read body error: %v", err))
	}
	var tunnel gatewaypkg.Tunnel
	if err := jsonx.Unmarshal(body, &tunnel); err != nil {
		return gatewaypkg.Tunnel{}, httppkg.NewError(http.StatusBadRequest, fmt.Sprintf("parse JSON error: %v", err))
	}
	tunnel.Name = strings.TrimSpace(tunnel.Name)
	tunnel.Remark = strings.TrimSpace(tunnel.Remark)
	tunnel.Protocol = strings.TrimSpace(tunnel.Protocol)
	tunnel.BindAddr = strings.TrimSpace(tunnel.BindAddr)
	tunnel.ClientKey = strings.TrimSpace(tunnel.ClientKey)
	tunnel.TargetHost = strings.TrimSpace(tunnel.TargetHost)
	if err := c.validateGatewayTunnelClient(tunnel); err != nil {
		return gatewaypkg.Tunnel{}, err
	}
	return tunnel, nil
}

func (c *Controller) validateGatewayTunnelClient(tunnel gatewaypkg.Tunnel) error {
	if c.clientRegistry != nil {
		client, ok := c.clientRegistry.GetByKey(strings.TrimSpace(tunnel.ClientKey))
		if !ok {
			return httppkg.NewError(http.StatusBadRequest, "selected gateway client not found")
		}
		if !client.HasStableClientID {
			return httppkg.NewError(http.StatusBadRequest, "selected gateway client must configure clientID")
		}
		if !client.AllowGatewayTunnels {
			return httppkg.NewError(http.StatusBadRequest, "selected gateway client does not allow gateway tunnels")
		}
	}
	return nil
}

func (c *Controller) validateGatewayTunnelImportPayload(tunnel gatewaypkg.Tunnel) error {
	if strings.TrimSpace(tunnel.ClientKey) == "" {
		return fmt.Errorf("clientKey is required")
	}
	// Import is designed for restore/idempotent replay. Unknown/offline client keys
	// are accepted and will stay pending until a matching client appears.
	return nil
}

func parseGatewayTunnelYAML(raw string) (gatewayTunnelExportResponse, error) {
	var payload gatewayTunnelExportResponse
	structErr := yaml.Unmarshal([]byte(raw), &payload)
	if structErr == nil {
		if payload.Tunnels != nil || payload.Version != 0 || payload.TunnelCount != 0 || payload.ExportedAt != 0 {
			return payload, nil
		}
	}

	var list []gatewayTunnelYAML
	listErr := yaml.Unmarshal([]byte(raw), &list)
	if listErr == nil {
		return gatewayTunnelExportResponse{
			Version:     1,
			TunnelCount: len(list),
			Tunnels:     list,
		}, nil
	}

	if structErr != nil {
		return gatewayTunnelExportResponse{}, fmt.Errorf("parse YAML error: %v", structErr)
	}
	return gatewayTunnelExportResponse{}, fmt.Errorf("parse YAML error: expected {tunnels: [...]} or a YAML list of tunnels")
}

func gatewayTunnelIdentity(clientKey, name string) string {
	return strings.TrimSpace(clientKey) + "\x00" + strings.TrimSpace(name)
}

func (c *Controller) getProxyStatsByType(proxyType string) (proxyInfos []*model.ProxyStatsInfo) {
	proxyStats := mem.StatsCollector.GetProxiesByType(proxyType)
	proxyInfos = make([]*model.ProxyStatsInfo, 0, len(proxyStats))
	for _, ps := range proxyStats {
		proxyInfo := &model.ProxyStatsInfo{
			User:     ps.User,
			ClientID: ps.ClientID,
		}
		if pxy, ok := c.pxyManager.GetByName(ps.Name); ok {
			proxyInfo.Conf = getConfFromConfigurer(pxy.GetConfigurer())
			proxyInfo.Status = "online"
		} else {
			proxyInfo.Status = "offline"
		}
		proxyInfo.Name = ps.Name
		proxyInfo.TodayTrafficIn = ps.TodayTrafficIn
		proxyInfo.TodayTrafficOut = ps.TodayTrafficOut
		proxyInfo.CurConns = ps.CurConns
		proxyInfo.LastStartTime = ps.LastStartTime
		proxyInfo.LastCloseTime = ps.LastCloseTime
		proxyInfos = append(proxyInfos, proxyInfo)
	}
	return
}

func (c *Controller) getProxyStatsByTypeAndName(proxyType string, proxyName string) (proxyInfo model.GetProxyStatsResp, code int, msg string) {
	proxyInfo.Name = proxyName
	ps := mem.StatsCollector.GetProxiesByTypeAndName(proxyType, proxyName)
	if ps == nil {
		code = 404
		msg = "no proxy info found"
	} else {
		proxyInfo.User = ps.User
		proxyInfo.ClientID = ps.ClientID
		if pxy, ok := c.pxyManager.GetByName(proxyName); ok {
			proxyInfo.Conf = getConfFromConfigurer(pxy.GetConfigurer())
			proxyInfo.Status = "online"
		} else {
			proxyInfo.Status = "offline"
		}
		proxyInfo.TodayTrafficIn = ps.TodayTrafficIn
		proxyInfo.TodayTrafficOut = ps.TodayTrafficOut
		proxyInfo.CurConns = ps.CurConns
		proxyInfo.LastStartTime = ps.LastStartTime
		proxyInfo.LastCloseTime = ps.LastCloseTime
		code = 200
	}

	return
}

func buildClientInfoResp(info registry.ClientInfo) model.ClientInfoResp {
	resp := model.ClientInfoResp{
		Key:                 info.Key,
		User:                info.User,
		ClientID:            info.ClientID(),
		RunID:               info.RunID,
		Version:             info.Version,
		Hostname:            info.Hostname,
		ClientIP:            info.IP,
		SelectedProtocol:    info.SelectedProtocol,
		AllowGatewayTunnels: info.AllowGatewayTunnels,
		HasStableClientID:   info.HasStableClientID,
		FirstConnectedAt:    toUnix(info.FirstConnectedAt),
		LastConnectedAt:     toUnix(info.LastConnectedAt),
		Online:              info.Online,
	}
	if !info.DisconnectedAt.IsZero() {
		resp.DisconnectedAt = info.DisconnectedAt.Unix()
	}
	return resp
}

func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func matchStatusFilter(online bool, filter string) bool {
	switch strings.ToLower(filter) {
	case "", "all":
		return true
	case "online":
		return online
	case "offline":
		return !online
	default:
		return true
	}
}

func getConfFromConfigurer(cfg v1.ProxyConfigurer) any {
	outBase := model.BaseOutConf{ProxyBaseConfig: *cfg.GetBaseConfig()}

	switch c := cfg.(type) {
	case *v1.TCPProxyConfig:
		return &model.TCPOutConf{BaseOutConf: outBase, RemotePort: c.RemotePort}
	case *v1.UDPProxyConfig:
		return &model.UDPOutConf{BaseOutConf: outBase, RemotePort: c.RemotePort}
	case *v1.HTTPProxyConfig:
		return &model.HTTPOutConf{
			BaseOutConf:       outBase,
			DomainConfig:      c.DomainConfig,
			Locations:         c.Locations,
			HostHeaderRewrite: c.HostHeaderRewrite,
		}
	case *v1.HTTPSProxyConfig:
		return &model.HTTPSOutConf{
			BaseOutConf:  outBase,
			DomainConfig: c.DomainConfig,
		}
	case *v1.TCPMuxProxyConfig:
		return &model.TCPMuxOutConf{
			BaseOutConf:     outBase,
			DomainConfig:    c.DomainConfig,
			Multiplexer:     c.Multiplexer,
			RouteByHTTPUser: c.RouteByHTTPUser,
		}
	case *v1.STCPProxyConfig:
		return &model.STCPOutConf{BaseOutConf: outBase}
	case *v1.XTCPProxyConfig:
		return &model.XTCPOutConf{BaseOutConf: outBase}
	}
	return outBase
}
