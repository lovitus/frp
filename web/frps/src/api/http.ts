// http.ts - Base HTTP client

class HTTPError extends Error {
  status: number
  statusText: string
  responseBody?: any

  constructor(
    status: number,
    statusText: string,
    message?: string,
    responseBody?: any,
  ) {
    super(message || statusText)
    this.status = status
    this.statusText = statusText
    this.responseBody = responseBody
  }
}

async function request<T>(url: string, options: RequestInit = {}): Promise<T> {
  const defaultOptions: RequestInit = {
    credentials: 'include',
  }

  const response = await fetch(url, { ...defaultOptions, ...options })

  if (!response.ok) {
    let detailMessage = `HTTP ${response.status}`
    let responseBody: any = undefined
    try {
      const contentType = response.headers.get('content-type') || ''
      if (contentType.includes('application/json')) {
        const data = await response.json()
        responseBody = data
        if (data && typeof data.msg === 'string' && data.msg.trim() !== '') {
          detailMessage = data.msg
        }
      } else {
        const text = await response.text()
        if (text.trim() !== '') {
          detailMessage = text.trim()
        }
      }
    } catch {
      // keep default message
    }
    throw new HTTPError(
      response.status,
      response.statusText,
      detailMessage,
      responseBody,
    )
  }

  // Handle empty response (e.g. 204 No Content)
  if (response.status === 204) {
    return {} as T
  }

  return response.json()
}

export const http = {
  get: <T>(url: string, options?: RequestInit) =>
    request<T>(url, { ...options, method: 'GET' }),
  post: <T>(url: string, body?: any, options?: RequestInit) =>
    request<T>(url, {
      ...options,
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...options?.headers },
      body: JSON.stringify(body),
    }),
  put: <T>(url: string, body?: any, options?: RequestInit) =>
    request<T>(url, {
      ...options,
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...options?.headers },
      body: JSON.stringify(body),
    }),
  delete: <T>(url: string, options?: RequestInit) =>
    request<T>(url, { ...options, method: 'DELETE' }),
}
