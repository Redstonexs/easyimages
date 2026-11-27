interface CacheEntry<T> {
  data: T
  updatedAt: number
}

const store = new Map<string, CacheEntry<unknown>>()
const inflight = new Map<string, Promise<unknown>>()

export function getCached<T>(key: string): T | undefined {
  return store.get(key)?.data as T | undefined
}

export function setCached<T>(key: string, data: T): void {
  store.set(key, { data, updatedAt: Date.now() })
}

export function invalidateCached(prefix?: string): void {
  if (!prefix) {
    store.clear()
    return
  }
  for (const key of store.keys()) {
    if (key.startsWith(prefix)) store.delete(key)
  }
}

/**
 * Fetches with in-flight deduplication and caches the resolved payload so
 * view switches and hover prefetches hydrate synchronously on frame 0.
 */
export function fetchCached<T>(key: string, fetcher: () => Promise<T>, maxAgeMs = 0): Promise<T> {
  if (maxAgeMs > 0) {
    const existing = store.get(key)
    if (existing && Date.now() - existing.updatedAt < maxAgeMs) {
      return Promise.resolve(existing.data as T)
    }
  }
  const pending = inflight.get(key)
  if (pending) return pending as Promise<T>

  const promise = fetcher()
    .then(data => {
      setCached(key, data)
      return data
    })
    .finally(() => {
      if (inflight.get(key) === promise) {
        inflight.delete(key)
      }
    })
  inflight.set(key, promise)
  return promise
}

export function prefetchCached<T>(key: string, fetcher: () => Promise<T>, maxAgeMs = 15_000): void {
  void fetchCached(key, fetcher, maxAgeMs).catch(() => {
    // Prefetch is best-effort; foreground views handle errors explicitly.
  })
}
