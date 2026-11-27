import { useEffect, useState } from 'react'

/**
 * Delays showing a skeleton/spinner so fast local responses (<160ms) transition
 * straight into content instead of flashing gray skeleton boxes.
 */
export function useDelayedLoading(loading: boolean, delayMs = 160): boolean {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!loading) {
      setVisible(false)
      return
    }
    const timer = window.setTimeout(() => setVisible(true), delayMs)
    return () => window.clearTimeout(timer)
  }, [loading, delayMs])

  return visible
}
