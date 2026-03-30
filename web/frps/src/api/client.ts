import { http } from './http'
import type { ClientInfoData } from '../types/client'
import type { GatewaySystemInfoData } from '../types/client-system'

export const getClients = () => {
  return http.get<ClientInfoData[]>('../api/clients')
}

export const getClient = (key: string) => {
  return http.get<ClientInfoData>(`../api/clients/${key}`)
}

export const getClientGatewaySystemInfo = (key: string) => {
  return http.get<GatewaySystemInfoData>(`../api/clients/${key}/gateway-system-info`)
}
