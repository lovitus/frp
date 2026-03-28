import { http } from './http'
import type { GatewayTunnelData, GatewayTunnelPayload } from '../types/gateway'

export const getGatewayTunnels = (refresh = true) => {
  return http.get<GatewayTunnelData[]>(
    `../api/gateway-tunnels?refresh=${refresh ? 'true' : 'false'}`,
  )
}

export const createGatewayTunnel = (payload: GatewayTunnelPayload) => {
  return http.post<GatewayTunnelData>('../api/gateway-tunnels', payload)
}

export const updateGatewayTunnel = (
  id: string,
  payload: GatewayTunnelPayload,
) => {
  return http.put<GatewayTunnelData>(`../api/gateway-tunnels/${id}`, payload)
}

export const deleteGatewayTunnel = (id: string) => {
  return http.delete<{ code: number; msg: string }>(`../api/gateway-tunnels/${id}`)
}
