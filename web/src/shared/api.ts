export async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'same-origin',
    ...init
  })
  if (!response.ok) {
    let message = `HTTP ${response.status}`
    try {
      const data = await response.json() as { message?: string; error?: string; detail?: string }
      message = data.message || data.error || data.detail || message
    } catch {
      // Keep the HTTP status fallback.
    }
    throw new Error(message)
  }
  if (response.status === 204) return undefined as T
  return await response.json() as T
}
