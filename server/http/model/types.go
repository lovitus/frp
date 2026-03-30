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

package model

import (
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/msg"
)

type ServerInfoResp struct {
	Version               string `json:"version"`
	BindPort              int    `json:"bindPort"`
	VhostHTTPPort         int    `json:"vhostHTTPPort"`
	VhostHTTPSPort        int    `json:"vhostHTTPSPort"`
	TCPMuxHTTPConnectPort int    `json:"tcpmuxHTTPConnectPort"`
	KCPBindPort           int    `json:"kcpBindPort"`
	QUICBindPort          int    `json:"quicBindPort"`
	SubdomainHost         string `json:"subdomainHost"`
	MaxPoolCount          int64  `json:"maxPoolCount"`
	MaxPortsPerClient     int64  `json:"maxPortsPerClient"`
	HeartBeatTimeout      int64  `json:"heartbeatTimeout"`
	AllowPortsStr         string `json:"allowPortsStr,omitempty"`
	TLSForce              bool   `json:"tlsForce,omitempty"`

	TotalTrafficIn  int64            `json:"totalTrafficIn"`
	TotalTrafficOut int64            `json:"totalTrafficOut"`
	CurConns        int64            `json:"curConns"`
	ClientCounts    int64            `json:"clientCounts"`
	ProxyTypeCounts map[string]int64 `json:"proxyTypeCount"`
}

type ClientInfoResp struct {
	Key                 string            `json:"key"`
	User                string            `json:"user"`
	ClientID            string            `json:"clientID"`
	RunID               string            `json:"runID"`
	Version             string            `json:"version,omitempty"`
	Hostname            string            `json:"hostname"`
	ClientIP            string            `json:"clientIP,omitempty"`
	Os                  string            `json:"os,omitempty"`
	Arch                string            `json:"arch,omitempty"`
	PoolCount           int               `json:"poolCount,omitempty"`
	LoginTimestamp      int64             `json:"loginTimestamp,omitempty"`
	Metas               map[string]string `json:"metas,omitempty"`
	SelectedProtocol    string            `json:"selectedProtocol,omitempty"`
	AllowGatewayTunnels bool              `json:"allowGatewayTunnels"`
	HasStableClientID   bool              `json:"hasStableClientID"`
	FirstConnectedAt    int64             `json:"firstConnectedAt"`
	LastConnectedAt     int64             `json:"lastConnectedAt"`
	DisconnectedAt      int64             `json:"disconnectedAt,omitempty"`
	Online              bool              `json:"online"`
}

type GatewaySystemGatewaySummary msg.GatewaySystemGatewaySummary

type GatewaySystemInfoResp struct {
	Key              string                       `json:"key"`
	DisplayName      string                       `json:"displayName"`
	ClientID         string                       `json:"clientID"`
	RunID            string                       `json:"runID"`
	Hostname         string                       `json:"hostname"`
	ObservedSourceIP string                       `json:"observedSourceIP,omitempty"`
	OS               string                       `json:"os,omitempty"`
	Arch             string                       `json:"arch,omitempty"`
	KernelVersion    string                       `json:"kernelVersion,omitempty"`
	Platform         string                       `json:"platform,omitempty"`
	PlatformVersion  string                       `json:"platformVersion,omitempty"`
	Timezone         string                       `json:"timezone,omitempty"`
	UptimeSeconds    uint64                       `json:"uptimeSeconds,omitempty"`
	CurrentUser      string                       `json:"currentUser,omitempty"`
	FRPCVersion      string                       `json:"frpcVersion,omitempty"`
	SelectedProtocol string                       `json:"selectedProtocol,omitempty"`
	DefaultRouteIP   string                       `json:"defaultRouteIP,omitempty"`
	CPUCount         int                          `json:"cpuCount,omitempty"`
	Load1            float64                      `json:"load1,omitempty"`
	Load5            float64                      `json:"load5,omitempty"`
	Load15           float64                      `json:"load15,omitempty"`
	MemoryTotal      uint64                       `json:"memoryTotal,omitempty"`
	MemoryUsed       uint64                       `json:"memoryUsed,omitempty"`
	MemoryAvailable  uint64                       `json:"memoryAvailable,omitempty"`
	SwapTotal        uint64                       `json:"swapTotal,omitempty"`
	SwapUsed         uint64                       `json:"swapUsed,omitempty"`
	DiskPath         string                       `json:"diskPath,omitempty"`
	DiskTotal        uint64                       `json:"diskTotal,omitempty"`
	DiskUsed         uint64                       `json:"diskUsed,omitempty"`
	FRPCPID          int32                        `json:"frpcPid,omitempty"`
	FRPCStartTime    int64                        `json:"frpcStartTime,omitempty"`
	Goroutines       int                          `json:"goroutines,omitempty"`
	CollectedAt      int64                        `json:"collectedAt,omitempty"`
	Interfaces       []msg.GatewaySystemInterface `json:"interfaces,omitempty"`
	TopMemoryProcs   []msg.GatewaySystemProcess   `json:"topMemoryProcs,omitempty"`
	Gateway          GatewaySystemGatewaySummary  `json:"gateway"`
	Metas            map[string]string            `json:"metas,omitempty"`
}

type BaseOutConf struct {
	v1.ProxyBaseConfig
}

type TCPOutConf struct {
	BaseOutConf
	RemotePort int `json:"remotePort"`
}

type TCPMuxOutConf struct {
	BaseOutConf
	v1.DomainConfig
	Multiplexer     string `json:"multiplexer"`
	RouteByHTTPUser string `json:"routeByHTTPUser"`
}

type UDPOutConf struct {
	BaseOutConf
	RemotePort int `json:"remotePort"`
}

type HTTPOutConf struct {
	BaseOutConf
	v1.DomainConfig
	Locations         []string `json:"locations"`
	HostHeaderRewrite string   `json:"hostHeaderRewrite"`
}

type HTTPSOutConf struct {
	BaseOutConf
	v1.DomainConfig
}

type STCPOutConf struct {
	BaseOutConf
}

type XTCPOutConf struct {
	BaseOutConf
}

// Get proxy info.
type ProxyStatsInfo struct {
	Name            string `json:"name"`
	Conf            any    `json:"conf"`
	User            string `json:"user,omitempty"`
	ClientID        string `json:"clientID,omitempty"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	CurConns        int64  `json:"curConns"`
	LastStartTime   string `json:"lastStartTime"`
	LastCloseTime   string `json:"lastCloseTime"`
	Status          string `json:"status"`
}

type GetProxyInfoResp struct {
	Proxies []*ProxyStatsInfo `json:"proxies"`
}

// Get proxy info by name.
type GetProxyStatsResp struct {
	Name            string `json:"name"`
	Conf            any    `json:"conf"`
	User            string `json:"user,omitempty"`
	ClientID        string `json:"clientID,omitempty"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	CurConns        int64  `json:"curConns"`
	LastStartTime   string `json:"lastStartTime"`
	LastCloseTime   string `json:"lastCloseTime"`
	Status          string `json:"status"`
}

// /api/traffic/:name
type GetProxyTrafficResp struct {
	Name       string  `json:"name"`
	TrafficIn  []int64 `json:"trafficIn"`
	TrafficOut []int64 `json:"trafficOut"`
}
