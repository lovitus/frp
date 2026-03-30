// Copyright 2016 fatedier, fatedier@gmail.com
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

package msg

import (
	"net"
	"reflect"
)

const (
	TypeLogin                       = 'o'
	TypeLoginResp                   = '1'
	TypeNewProxy                    = 'p'
	TypeNewProxyResp                = '2'
	TypeCloseProxy                  = 'c'
	TypeNewWorkConn                 = 'w'
	TypeReqWorkConn                 = 'r'
	TypeStartWorkConn               = 's'
	TypeNewVisitorConn              = 'v'
	TypeNewVisitorConnResp          = '3'
	TypePing                        = 'h'
	TypePong                        = '4'
	TypeUDPPacket                   = 'u'
	TypeGatewayTunnelsSync          = 'g'
	TypeGatewayTunnelStatusRequest  = 'j'
	TypeGatewayTunnelStatusResponse = 'k'
	TypeGatewaySystemInfoRequest    = 'l'
	TypeGatewaySystemInfoResponse   = 'q'
	TypeNatHoleVisitor              = 'i'
	TypeNatHoleClient               = 'n'
	TypeNatHoleResp                 = 'm'
	TypeNatHoleSid                  = '5'
	TypeNatHoleReport               = '6'
)

var msgTypeMap = map[byte]any{
	TypeLogin:                       Login{},
	TypeLoginResp:                   LoginResp{},
	TypeNewProxy:                    NewProxy{},
	TypeNewProxyResp:                NewProxyResp{},
	TypeCloseProxy:                  CloseProxy{},
	TypeNewWorkConn:                 NewWorkConn{},
	TypeReqWorkConn:                 ReqWorkConn{},
	TypeStartWorkConn:               StartWorkConn{},
	TypeNewVisitorConn:              NewVisitorConn{},
	TypeNewVisitorConnResp:          NewVisitorConnResp{},
	TypePing:                        Ping{},
	TypePong:                        Pong{},
	TypeUDPPacket:                   UDPPacket{},
	TypeGatewayTunnelsSync:          GatewayTunnelsSync{},
	TypeGatewayTunnelStatusRequest:  GatewayTunnelStatusRequest{},
	TypeGatewayTunnelStatusResponse: GatewayTunnelStatusResponse{},
	TypeGatewaySystemInfoRequest:    GatewaySystemInfoRequest{},
	TypeGatewaySystemInfoResponse:   GatewaySystemInfoResponse{},
	TypeNatHoleVisitor:              NatHoleVisitor{},
	TypeNatHoleClient:               NatHoleClient{},
	TypeNatHoleResp:                 NatHoleResp{},
	TypeNatHoleSid:                  NatHoleSid{},
	TypeNatHoleReport:               NatHoleReport{},
}

var TypeNameNatHoleResp = reflect.TypeFor[NatHoleResp]().Name()

type ClientSpec struct {
	// Due to the support of VirtualClient, frps needs to know the client type in order to
	// differentiate the processing logic.
	// Optional values: ssh-tunnel
	Type string `json:"type,omitempty"`
	// If the value is true, the client will not require authentication.
	AlwaysAuthPass bool `json:"always_auth_pass,omitempty"`
}

// When frpc start, client send this message to login to server.
type Login struct {
	Version      string            `json:"version,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	Os           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	User         string            `json:"user,omitempty"`
	PrivilegeKey string            `json:"privilege_key,omitempty"`
	Timestamp    int64             `json:"timestamp,omitempty"`
	RunID        string            `json:"run_id,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	Metas        map[string]string `json:"metas,omitempty"`

	// Currently only effective for VirtualClient.
	ClientSpec ClientSpec `json:"client_spec,omitempty"`

	// Some global configures.
	PoolCount int `json:"pool_count,omitempty"`

	SelectedProtocol    string `json:"selected_protocol,omitempty"`
	AllowGatewayTunnels bool   `json:"allow_gateway_tunnels,omitempty"`
}

type LoginResp struct {
	Version string `json:"version,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// When frpc login success, send this message to frps for running a new proxy.
type NewProxy struct {
	ProxyName          string            `json:"proxy_name,omitempty"`
	ProxyType          string            `json:"proxy_type,omitempty"`
	UseEncryption      bool              `json:"use_encryption,omitempty"`
	UseCompression     bool              `json:"use_compression,omitempty"`
	BandwidthLimit     string            `json:"bandwidth_limit,omitempty"`
	BandwidthLimitMode string            `json:"bandwidth_limit_mode,omitempty"`
	Group              string            `json:"group,omitempty"`
	GroupKey           string            `json:"group_key,omitempty"`
	Metas              map[string]string `json:"metas,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`

	// tcp and udp only
	RemotePort int `json:"remote_port,omitempty"`

	// http and https only
	CustomDomains     []string          `json:"custom_domains,omitempty"`
	SubDomain         string            `json:"subdomain,omitempty"`
	Locations         []string          `json:"locations,omitempty"`
	HTTPUser          string            `json:"http_user,omitempty"`
	HTTPPwd           string            `json:"http_pwd,omitempty"`
	HostHeaderRewrite string            `json:"host_header_rewrite,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	ResponseHeaders   map[string]string `json:"response_headers,omitempty"`
	RouteByHTTPUser   string            `json:"route_by_http_user,omitempty"`

	// stcp, sudp, xtcp
	Sk         string   `json:"sk,omitempty"`
	AllowUsers []string `json:"allow_users,omitempty"`

	// tcpmux
	Multiplexer string `json:"multiplexer,omitempty"`
}

type NewProxyResp struct {
	ProxyName  string `json:"proxy_name,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	Error      string `json:"error,omitempty"`
}

type CloseProxy struct {
	ProxyName string `json:"proxy_name,omitempty"`
}

type NewWorkConn struct {
	RunID        string `json:"run_id,omitempty"`
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type ReqWorkConn struct{}

type StartWorkConn struct {
	ProxyName string `json:"proxy_name,omitempty"`
	SrcAddr   string `json:"src_addr,omitempty"`
	DstAddr   string `json:"dst_addr,omitempty"`
	SrcPort   uint16 `json:"src_port,omitempty"`
	DstPort   uint16 `json:"dst_port,omitempty"`
	Error     string `json:"error,omitempty"`
}

type NewVisitorConn struct {
	RunID          string `json:"run_id,omitempty"`
	ProxyName      string `json:"proxy_name,omitempty"`
	SignKey        string `json:"sign_key,omitempty"`
	Timestamp      int64  `json:"timestamp,omitempty"`
	UseEncryption  bool   `json:"use_encryption,omitempty"`
	UseCompression bool   `json:"use_compression,omitempty"`
}

type NewVisitorConnResp struct {
	ProxyName string `json:"proxy_name,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Ping struct {
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type Pong struct {
	Error string `json:"error,omitempty"`
}

type UDPPacket struct {
	Content    []byte       `json:"c,omitempty"`
	LocalAddr  *net.UDPAddr `json:"l,omitempty"`
	RemoteAddr *net.UDPAddr `json:"r,omitempty"`
}

type GatewayTunnelConfig struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Remark     string `json:"remark,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	BindAddr   string `json:"bind_addr,omitempty"`
	ListenPort int    `json:"listen_port,omitempty"`
	TargetHost string `json:"target_host,omitempty"`
	TargetPort int    `json:"target_port,omitempty"`
}

type GatewayTunnelsSync struct {
	Tunnels []GatewayTunnelConfig `json:"tunnels,omitempty"`
}

type GatewayTunnelStatusRequest struct {
	RequestID string   `json:"request_id,omitempty"`
	TunnelIDs []string `json:"tunnel_ids,omitempty"`
}

type GatewayTunnelStatus struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

type GatewayTunnelStatusResponse struct {
	RequestID string                `json:"request_id,omitempty"`
	Statuses  []GatewayTunnelStatus `json:"statuses,omitempty"`
}

type GatewaySystemInfoRequest struct {
	RequestID string `json:"request_id,omitempty"`
}

type GatewaySystemInterface struct {
	Name      string   `json:"name,omitempty"`
	Flags     []string `json:"flags,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
}

type GatewaySystemProcess struct {
	PID           int32   `json:"pid,omitempty"`
	Name          string  `json:"name,omitempty"`
	MemoryRSS     uint64  `json:"memory_rss,omitempty"`
	MemoryPercent float32 `json:"memory_percent,omitempty"`
}

type GatewaySystemGatewaySummary struct {
	Enabled       bool   `json:"enabled,omitempty"`
	TunnelCount   int    `json:"tunnel_count,omitempty"`
	OnlineCount   int    `json:"online_count,omitempty"`
	PendingCount  int    `json:"pending_count,omitempty"`
	DisabledCount int    `json:"disabled_count,omitempty"`
	LastApplyErr  string `json:"last_apply_err,omitempty"`
}

type GatewaySystemInfo struct {
	CollectedAt      int64                       `json:"collected_at,omitempty"`
	Hostname         string                      `json:"hostname,omitempty"`
	OS               string                      `json:"os,omitempty"`
	Arch             string                      `json:"arch,omitempty"`
	KernelVersion    string                      `json:"kernel_version,omitempty"`
	Platform         string                      `json:"platform,omitempty"`
	PlatformVersion  string                      `json:"platform_version,omitempty"`
	Timezone         string                      `json:"timezone,omitempty"`
	UptimeSeconds    uint64                      `json:"uptime_seconds,omitempty"`
	CurrentUser      string                      `json:"current_user,omitempty"`
	FRPCVersion      string                      `json:"frpc_version,omitempty"`
	ClientID         string                      `json:"client_id,omitempty"`
	RunID            string                      `json:"run_id,omitempty"`
	SelectedProtocol string                      `json:"selected_protocol,omitempty"`
	DefaultRouteIP   string                      `json:"default_route_ip,omitempty"`
	CPUCount         int                         `json:"cpu_count,omitempty"`
	Load1            float64                     `json:"load_1,omitempty"`
	Load5            float64                     `json:"load_5,omitempty"`
	Load15           float64                     `json:"load_15,omitempty"`
	MemoryTotal      uint64                      `json:"memory_total,omitempty"`
	MemoryUsed       uint64                      `json:"memory_used,omitempty"`
	MemoryAvailable  uint64                      `json:"memory_available,omitempty"`
	SwapTotal        uint64                      `json:"swap_total,omitempty"`
	SwapUsed         uint64                      `json:"swap_used,omitempty"`
	DiskPath         string                      `json:"disk_path,omitempty"`
	DiskTotal        uint64                      `json:"disk_total,omitempty"`
	DiskUsed         uint64                      `json:"disk_used,omitempty"`
	FRPCPID          int32                       `json:"frpc_pid,omitempty"`
	FRPCStartTime    int64                       `json:"frpc_start_time,omitempty"`
	Goroutines       int                         `json:"goroutines,omitempty"`
	Interfaces       []GatewaySystemInterface    `json:"interfaces,omitempty"`
	TopMemoryProcs   []GatewaySystemProcess      `json:"top_memory_procs,omitempty"`
	Gateway          GatewaySystemGatewaySummary `json:"gateway,omitempty"`
	Metas            map[string]string           `json:"metas,omitempty"`
}

type GatewaySystemInfoResponse struct {
	RequestID string            `json:"request_id,omitempty"`
	Info      GatewaySystemInfo `json:"info,omitempty"`
}

type NatHoleVisitor struct {
	TransactionID string   `json:"transaction_id,omitempty"`
	ProxyName     string   `json:"proxy_name,omitempty"`
	PreCheck      bool     `json:"pre_check,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	SignKey       string   `json:"sign_key,omitempty"`
	Timestamp     int64    `json:"timestamp,omitempty"`
	MappedAddrs   []string `json:"mapped_addrs,omitempty"`
	AssistedAddrs []string `json:"assisted_addrs,omitempty"`
}

type NatHoleClient struct {
	TransactionID string   `json:"transaction_id,omitempty"`
	ProxyName     string   `json:"proxy_name,omitempty"`
	Sid           string   `json:"sid,omitempty"`
	MappedAddrs   []string `json:"mapped_addrs,omitempty"`
	AssistedAddrs []string `json:"assisted_addrs,omitempty"`
}

type PortsRange struct {
	From int `json:"from,omitempty"`
	To   int `json:"to,omitempty"`
}

type NatHoleDetectBehavior struct {
	Role              string       `json:"role,omitempty"` // sender or receiver
	Mode              int          `json:"mode,omitempty"` // 0, 1, 2...
	TTL               int          `json:"ttl,omitempty"`
	SendDelayMs       int          `json:"send_delay_ms,omitempty"`
	ReadTimeoutMs     int          `json:"read_timeout,omitempty"`
	CandidatePorts    []PortsRange `json:"candidate_ports,omitempty"`
	SendRandomPorts   int          `json:"send_random_ports,omitempty"`
	ListenRandomPorts int          `json:"listen_random_ports,omitempty"`
}

type NatHoleResp struct {
	TransactionID  string                `json:"transaction_id,omitempty"`
	Sid            string                `json:"sid,omitempty"`
	Protocol       string                `json:"protocol,omitempty"`
	CandidateAddrs []string              `json:"candidate_addrs,omitempty"`
	AssistedAddrs  []string              `json:"assisted_addrs,omitempty"`
	DetectBehavior NatHoleDetectBehavior `json:"detect_behavior,omitempty"`
	Error          string                `json:"error,omitempty"`
}

type NatHoleSid struct {
	TransactionID string `json:"transaction_id,omitempty"`
	Sid           string `json:"sid,omitempty"`
	Response      bool   `json:"response,omitempty"`
	Nonce         string `json:"nonce,omitempty"`
}

type NatHoleReport struct {
	Sid     string `json:"sid,omitempty"`
	Success bool   `json:"success,omitempty"`
}
