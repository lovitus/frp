export interface ClientInfoData {
  key: string
  user: string
  clientID: string
  runID: string
  version?: string
  hostname: string
  clientIP?: string
  os?: string
  arch?: string
  poolCount?: number
  loginTimestamp?: number
  selectedProtocol?: string
  allowGatewayTunnels: boolean
  hasStableClientID: boolean
  metas?: Record<string, string>
  firstConnectedAt: number
  lastConnectedAt: number
  disconnectedAt?: number
  online: boolean
}
