import { http } from './http'
import type {
  GatewayTunnelData,
  GatewayTunnelExportResponse,
  GatewayTunnelImportPayload,
  GatewayTunnelImportResponse,
  GatewayTunnelPayload,
} from '../types/gateway'

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

export const exportGatewayTunnels = () => {
  return http.get<GatewayTunnelExportResponse>('../api/gateway-tunnels/export')
}

export const importGatewayTunnels = (payload: GatewayTunnelImportPayload) => {
  return http.post<GatewayTunnelImportResponse>(
    '../api/gateway-tunnels/import',
    payload,
  )
}
