export type GatewayProtocol = 'tcp' | 'udp'

export interface GatewayTunnelData {
  id: string
  name: string
  remark?: string
  protocol: GatewayProtocol
  bindAddr: string
  listenPort: number
  clientKey: string
  targetHost: string
  targetPort: number
  status: string
  message?: string
  remoteAddr?: string
  updatedAt: string
}

export interface GatewayTunnelPayload {
  name: string
  remark?: string
  protocol: GatewayProtocol
  bindAddr: string
  listenPort: number
  clientKey: string
  targetHost: string
  targetPort: number
}
