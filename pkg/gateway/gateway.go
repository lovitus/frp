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
	AnnotationBindAddrKey         = "frp/gateway-bind-addr"

	StatusPending           = "pending"
	StatusOnline            = "online"
	StatusClientOffline     = "client-offline"
	StatusDisabled          = "disabled"
	StatusInvalidConfig     = "invalid-config"
	StatusApplyFailed       = "apply-failed"
	StatusRegisterFailed    = "register-failed"
	StatusTargetInvalid     = "target-invalid"
	StatusTargetUnreachable = "target-unreachable"
)

type Tunnel struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Remark     string    `json:"remark,omitempty"`
	Protocol   string    `json:"protocol"`
	BindAddr   string    `json:"bindAddr"`
	ListenPort int       `json:"listenPort"`
	ClientKey  string    `json:"clientKey"`
	TargetHost string    `json:"targetHost"`
	TargetPort int       `json:"targetPort"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	RemoteAddr string    `json:"remoteAddr,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
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
