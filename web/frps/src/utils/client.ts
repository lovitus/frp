import { formatDistanceToNow } from './format'
import type { ClientInfoData } from '../types/client'

export class Client {
  key: string
  user: string
  clientID: string
  runID: string
  version: string
  hostname: string
  ip: string
  os: string
  arch: string
  poolCount: number
  loginTimestamp?: Date
  selectedProtocol: string
  allowGatewayTunnels: boolean
  hasStableClientID: boolean
  metas: Map<string, string>
  firstConnectedAt: Date
  lastConnectedAt: Date
  disconnectedAt?: Date
  online: boolean

  constructor(data: ClientInfoData) {
    this.key = data.key
    this.user = data.user
    this.clientID = data.clientID
    this.runID = data.runID
    this.version = data.version || ''
    this.hostname = data.hostname
    this.ip = data.clientIP || ''
    this.os = data.os || ''
    this.arch = data.arch || ''
    this.poolCount = data.poolCount || 0
    if (data.loginTimestamp && data.loginTimestamp > 0) {
      this.loginTimestamp = new Date(data.loginTimestamp * 1000)
    }
    this.selectedProtocol = data.selectedProtocol || ''
    this.allowGatewayTunnels = data.allowGatewayTunnels
    this.hasStableClientID = data.hasStableClientID
    this.metas = new Map<string, string>()
    if (data.metas) {
      for (const [key, value] of Object.entries(data.metas)) {
        this.metas.set(key, value)
      }
    }
    this.firstConnectedAt = new Date(data.firstConnectedAt * 1000)
    this.lastConnectedAt = new Date(data.lastConnectedAt * 1000)
    if (data.disconnectedAt && data.disconnectedAt > 0) {
      this.disconnectedAt = new Date(data.disconnectedAt * 1000)
    }
    this.online = data.online
  }

  get displayName(): string {
    if (this.clientID) {
      return this.user ? `${this.user}.${this.clientID}` : this.clientID
    }
    return this.runID
  }

  get shortRunId(): string {
    return this.runID.substring(0, 8)
  }

  get platformLabel(): string {
    return [this.os, this.arch].filter(Boolean).join(' / ')
  }

  get loginAtAgo(): string {
    if (!this.loginTimestamp) return ''
    return formatDistanceToNow(this.loginTimestamp)
  }

  get firstConnectedAgo(): string {
    return formatDistanceToNow(this.firstConnectedAt)
  }

  get lastConnectedAgo(): string {
    return formatDistanceToNow(this.lastConnectedAt)
  }

  get disconnectedAgo(): string {
    if (!this.disconnectedAt) return ''
    return formatDistanceToNow(this.disconnectedAt)
  }

  get statusColor(): string {
    return this.online ? 'success' : 'danger'
  }

  get metasArray(): Array<{ key: string; value: string }> {
    const arr: Array<{ key: string; value: string }> = []
    this.metas.forEach((value, key) => {
      arr.push({ key, value })
    })
    return arr
  }

  matchesFilter(searchText: string): boolean {
    const search = searchText.toLowerCase()
    return (
      this.key.toLowerCase().includes(search) ||
      this.user.toLowerCase().includes(search) ||
      this.clientID.toLowerCase().includes(search) ||
      this.runID.toLowerCase().includes(search) ||
      this.hostname.toLowerCase().includes(search)
    )
  }
}
