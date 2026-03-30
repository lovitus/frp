package gateway

import (
	"net"
	"strings"
	"time"
)

const (
	AnnotationSourceKey           = "frp/runtime-source"
	AnnotationSourceGatewayTunnel = "gateway-tunnel"
	AnnotationTunnelIDKey         = "frp/gateway-tunnel-id"
	AnnotationTunnelNameKey       = "frp/gateway-tunnel-name"
	AnnotationTunnelRemarkKey     = "frp/gateway-tunnel-remark"
	AnnotationTargetTypeKey       = "frp/gateway-target-type"
	AnnotationTargetHostKey       = "frp/gateway-target-host"
	AnnotationTargetPortKey       = "frp/gateway-target-port"
	AnnotationBindAddrKey         = "frp/gateway-bind-addr"

	TargetTypeDirect      = "direct"
	TargetTypeSSProxy     = "ss_proxy"
	TargetTypeSocks5Proxy = "socks5_proxy"

	ValidityUnitPermanent = "permanent"
	ValidityUnitHour      = "h"
	ValidityUnitDay       = "d"

	StatusPending           = "pending"
	StatusOnline            = "online"
	StatusClientOffline     = "client-offline"
	StatusDisabled          = "disabled"
	StatusExpired           = "expired"
	StatusInvalidConfig     = "invalid-config"
	StatusApplyFailed       = "apply-failed"
	StatusRegisterFailed    = "register-failed"
	StatusTargetInvalid     = "target-invalid"
	StatusTargetUnreachable = "target-unreachable"
)

type Tunnel struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Remark        string    `json:"remark,omitempty"`
	Protocol      string    `json:"protocol"`
	BindAddr      string    `json:"bindAddr"`
	ListenPort    int       `json:"listenPort"`
	ClientKey     string    `json:"clientKey"`
	TargetType    string    `json:"targetType,omitempty"`
	TargetHost    string    `json:"targetHost"`
	TargetPort    int       `json:"targetPort"`
	SSMethod      string    `json:"ssMethod,omitempty"`
	SSPassword    string    `json:"ssPassword,omitempty"`
	Socks5Auth    bool      `json:"socks5Auth,omitempty"`
	Socks5User    string    `json:"socks5User,omitempty"`
	Socks5Pass    string    `json:"socks5Pass,omitempty"`
	ValidityValue int       `json:"validityValue,omitempty"`
	ValidityUnit  string    `json:"validityUnit,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	RemoteAddr    string    `json:"remoteAddr,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func ProxyName(id string) string {
	return "__gateway_tunnel_" + strings.TrimSpace(id)
}

func IsManagedRuntime(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	return annotations[AnnotationSourceKey] == AnnotationSourceGatewayTunnel
}

func ResolveBindAddr(annotations map[string]string, defaultBindAddr string) string {
	if !IsManagedRuntime(annotations) {
		return defaultBindAddr
	}

	value := strings.TrimSpace(strings.Trim(annotations[AnnotationBindAddrKey], "[]"))
	if value == "" {
		return defaultBindAddr
	}
	if net.ParseIP(value) == nil {
		return defaultBindAddr
	}
	return value
}
