export interface GatewaySystemInterfaceData {
  name: string
  flags?: string[]
  addresses?: string[]
}

export interface GatewaySystemProcessData {
  pid: number
  name: string
  memory_rss?: number
  memory_percent?: number
}

export interface GatewaySystemGatewaySummaryData {
  enabled: boolean
  tunnel_count: number
  online_count: number
  pending_count: number
  disabled_count: number
  last_apply_err?: string
}

export interface GatewaySystemInfoData {
  key: string
  displayName: string
  clientID: string
  runID: string
  hostname: string
  observedSourceIP?: string
  os?: string
  arch?: string
  kernelVersion?: string
  platform?: string
  platformVersion?: string
  timezone?: string
  uptimeSeconds?: number
  currentUser?: string
  frpcVersion?: string
  selectedProtocol?: string
  defaultRouteIP?: string
  cpuCount?: number
  load1?: number
  load5?: number
  load15?: number
  memoryTotal?: number
  memoryUsed?: number
  memoryAvailable?: number
  swapTotal?: number
  swapUsed?: number
  diskPath?: string
  diskTotal?: number
  diskUsed?: number
  frpcPid?: number
  frpcStartTime?: number
  goroutines?: number
  collectedAt?: number
  interfaces?: GatewaySystemInterfaceData[]
  topMemoryProcs?: GatewaySystemProcessData[]
  gateway: GatewaySystemGatewaySummaryData
  metas?: Record<string, string>
}
