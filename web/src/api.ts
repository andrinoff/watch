import type { Item, Library } from './types'

async function request<T>(method: string, path: string): Promise<T> {
  const res = await fetch(path, { method })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const data = await res.json()
      if (data?.error) message = data.error
    } catch {
      /* keep the status line */
    }
    throw new Error(message)
  }
  return res.json() as Promise<T>
}

export const api = {
  library: () => request<Library>('GET', '/api/library'),
  rescan: () => request<{ items: number }>('POST', '/api/rescan'),
}

export const mediaUrl = (id: string) => `/api/media/${id}`
export const downloadUrl = (id: string) => `/api/media/${id}?download=1`
export const posterUrl = (id: string) => `/api/poster/${id}`

export function formatSize(bytes: number): string {
  if (!bytes) return ''
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(0)} MB`
  return `${(bytes / 1024).toFixed(0)} KB`
}

export function formatDuration(seconds: number): string {
  if (!seconds) return ''
  const total = Math.round(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m > 0) return `${m}m ${String(s).padStart(2, '0')}s`
  return `${s}s`
}

export function formatClock(seconds: number): string {
  const total = Math.round(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

export function displayMeta(item: Item): string {
  const parts: string[] = []
  if (item.year) parts.push(String(item.year))
  if (item.duration) parts.push(formatDuration(item.duration))
  if (item.size) parts.push(formatSize(item.size))
  return parts.join('  ·  ')
}
