package client

import (
	"context"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	gopsmem "github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/version"
)

const gatewaySystemInfoProcessLimit = 10

func (m *GatewayTunnelManager) BuildGatewaySystemInfoResponse(requestID string) *msg.GatewaySystemInfoResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return &msg.GatewaySystemInfoResponse{
		RequestID: requestID,
		Info:      m.collectGatewaySystemInfo(ctx),
	}
}

func (m *GatewayTunnelManager) collectGatewaySystemInfo(ctx context.Context) msg.GatewaySystemInfo {
	info := msg.GatewaySystemInfo{
		CollectedAt:      time.Now().Unix(),
		Hostname:         currentHostname(),
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		Timezone:         time.Now().Location().String(),
		CurrentUser:      currentUsername(),
		FRPCVersion:      version.Full(),
		ClientID:         m.service.common.ClientID,
		RunID:            m.service.runID,
		SelectedProtocol: m.service.getSelectedProtocol(),
		CPUCount:         runtime.NumCPU(),
		FRPCPID:          int32(os.Getpid()),
		Goroutines:       runtime.NumGoroutine(),
		Interfaces:       collectNetworkInterfaces(),
		DefaultRouteIP:   detectDefaultRouteIP(),
		Gateway:          m.buildGatewaySummary(),
		Metas:            cloneGatewayMetas(m.service.common.Metadatas),
	}

	if hostInfo, err := host.InfoWithContext(ctx); err == nil {
		if info.Hostname == "" {
			info.Hostname = hostInfo.Hostname
		}
		if hostInfo.OS != "" {
			info.OS = hostInfo.OS
		}
		if hostInfo.KernelVersion != "" {
			info.KernelVersion = hostInfo.KernelVersion
		}
		info.Platform = hostInfo.Platform
		info.PlatformVersion = hostInfo.PlatformVersion
		info.UptimeSeconds = hostInfo.Uptime
	}

	if avg, err := load.AvgWithContext(ctx); err == nil {
		info.Load1 = avg.Load1
		info.Load5 = avg.Load5
		info.Load15 = avg.Load15
	}

	if vm, err := gopsmem.VirtualMemoryWithContext(ctx); err == nil {
		info.MemoryTotal = vm.Total
		info.MemoryUsed = vm.Used
		info.MemoryAvailable = vm.Available
	}

	if sm, err := gopsmem.SwapMemoryWithContext(ctx); err == nil {
		info.SwapTotal = sm.Total
		info.SwapUsed = sm.Used
	}

	if p, err := process.NewProcessWithContext(ctx, info.FRPCPID); err == nil {
		if startedAt, err := p.CreateTimeWithContext(ctx); err == nil && startedAt > 0 {
			info.FRPCStartTime = startedAt / 1000
		}
	}

	if diskPath, total, used := collectDiskUsage(ctx); total > 0 {
		info.DiskPath = diskPath
		info.DiskTotal = total
		info.DiskUsed = used
	}

	info.TopMemoryProcs = collectTopMemoryProcesses(ctx, gatewaySystemInfoProcessLimit)
	return info
}

func (m *GatewayTunnelManager) buildGatewaySummary() msg.GatewaySystemGatewaySummary {
	m.mu.RLock()
	selected := make([]*gatewayTunnelRuntime, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		selected = append(selected, tunnel)
	}
	applyErr := m.lastApplyErr
	enabled := m.enabled
	m.mu.RUnlock()

	summary := msg.GatewaySystemGatewaySummary{
		Enabled:      enabled,
		TunnelCount:  len(selected),
		LastApplyErr: applyErr,
	}
	for _, tunnel := range selected {
		status := msg.GatewayTunnelStatus{}
		switch {
		case !enabled:
			summary.DisabledCount++
		case tunnel.validationErr != "":
			summary.PendingCount++
		default:
			status = m.fillGatewayTunnelRuntimeStatus(status, tunnel, applyErr)
			switch status.Status {
			case gateway.StatusOnline:
				summary.OnlineCount++
			case gateway.StatusDisabled:
				summary.DisabledCount++
			default:
				summary.PendingCount++
			}
		}
	}
	return summary
}

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Name
}

func currentHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

func collectNetworkInterfaces() []msg.GatewaySystemInterface {
	items, err := net.Interfaces()
	if err != nil {
		return nil
	}
	result := make([]msg.GatewaySystemInterface, 0, len(items))
	for _, item := range items {
		addrs, err := item.Addrs()
		if err != nil {
			continue
		}
		info := msg.GatewaySystemInterface{
			Name:      item.Name,
			Flags:     interfaceFlags(item.Flags),
			Addresses: make([]string, 0, len(addrs)),
		}
		for _, addr := range addrs {
			info.Addresses = append(info.Addresses, addr.String())
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func interfaceFlags(flags net.Flags) []string {
	labels := make([]string, 0, 4)
	if flags&net.FlagUp != 0 {
		labels = append(labels, "up")
	}
	if flags&net.FlagRunning != 0 {
		labels = append(labels, "running")
	}
	if flags&net.FlagLoopback != 0 {
		labels = append(labels, "loopback")
	}
	if flags&net.FlagBroadcast != 0 {
		labels = append(labels, "broadcast")
	}
	if flags&net.FlagPointToPoint != 0 {
		labels = append(labels, "p2p")
	}
	if flags&net.FlagMulticast != 0 {
		labels = append(labels, "multicast")
	}
	return labels
}

func detectDefaultRouteIP() string {
	targets := []string{"192.0.2.1:80", "[2001:db8::1]:80"}
	for _, target := range targets {
		conn, err := net.Dial("udp", target)
		if err != nil {
			continue
		}
		addr := conn.LocalAddr().String()
		_ = conn.Close()
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			return host
		}
		return addr
	}
	return ""
}

func collectDiskUsage(ctx context.Context) (string, uint64, uint64) {
	path := "."
	if wd, err := os.Getwd(); err == nil && wd != "" {
		path = wd
	}
	usage, err := disk.UsageWithContext(ctx, path)
	if err == nil {
		return path, usage.Total, usage.Used
	}
	cleanPath := filepath.VolumeName(path)
	if cleanPath == "" {
		cleanPath = string(os.PathSeparator)
	}
	usage, err = disk.UsageWithContext(ctx, cleanPath)
	if err != nil {
		return "", 0, 0
	}
	return cleanPath, usage.Total, usage.Used
}

func collectTopMemoryProcesses(ctx context.Context, limit int) []msg.GatewaySystemProcess {
	pids, err := process.PidsWithContext(ctx)
	if err != nil {
		return nil
	}

	items := make([]msg.GatewaySystemProcess, 0, len(pids))
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		p, err := process.NewProcessWithContext(ctx, pid)
		if err != nil {
			continue
		}
		memInfo, err := p.MemoryInfoWithContext(ctx)
		if err != nil || memInfo == nil || memInfo.RSS == 0 {
			continue
		}
		name, _ := p.NameWithContext(ctx)
		memPercent, _ := p.MemoryPercentWithContext(ctx)
		items = append(items, msg.GatewaySystemProcess{
			PID:           pid,
			Name:          strings.TrimSpace(name),
			MemoryRSS:     memInfo.RSS,
			MemoryPercent: memPercent,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].MemoryRSS == items[j].MemoryRSS {
			return items[i].PID < items[j].PID
		}
		return items[i].MemoryRSS > items[j].MemoryRSS
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func cloneGatewayMetas(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
