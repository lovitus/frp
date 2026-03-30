export type GatewayProtocol = 'tcp' | 'udp'
export type GatewayTargetType = 'direct' | 'ss_proxy' | 'socks5_proxy'
export type GatewayValidityUnit = 'permanent' | 'h' | 'd'

export interface GatewayTunnelData {
  id: string
  name: string
  remark?: string
  protocol: GatewayProtocol
  bindAddr: string
  listenPort: number
  clientKey: string
  targetType?: GatewayTargetType
  targetHost: string
  targetPort: number
  ssMethod?: string
  ssPassword?: string
  socks5Auth?: boolean
  socks5User?: string
  socks5Pass?: string
  validityValue?: number
  validityUnit?: GatewayValidityUnit
  expiresAt?: string
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
  targetType?: GatewayTargetType
  targetHost: string
  targetPort: number
  ssMethod?: string
  ssPassword?: string
  socks5Auth?: boolean
  socks5User?: string
  socks5Pass?: string
  validityValue?: number
  validityUnit?: GatewayValidityUnit
}

export interface GatewayTunnelExportResponse {
  yaml: string
}

export interface GatewayTunnelImportPayload {
  yaml: string
}

export interface GatewayTunnelImportResponse {
  total: number
  created: number
  updated: number
}
