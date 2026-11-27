import { useEffect, useRef, useState } from 'react'
import { ImageOff } from 'lucide-react'
import { cn } from '@/lib/utils'

interface FadeImageProps {
  src: string
  fallbackSrc?: string
  alt: string
  className?: string
  fallbackLabel?: string
  iconClassName?: string
  loading?: 'lazy' | 'eager'
}

/**
 * Smoothly reveals decoded images with an optional fallback URL (e.g. thumbnail
 * -> original) before showing a clean monochrome broken-image placeholder.
 */
export function FadeImage({
  src,
  fallbackSrc,
  alt,
  className,
  fallbackLabel,
  iconClassName = 'size-4',
  loading = 'lazy'
}: FadeImageProps) {
  const imgRef = useRef<HTMLImageElement>(null)
  const [activeSrc, setActiveSrc] = useState(src)
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setActiveSrc(src)
    setFailed(false)
    if (imgRef.current?.complete && imgRef.current.naturalWidth > 0 && imgRef.current.getAttribute('src') === src) {
      setLoaded(true)
    } else {
      setLoaded(false)
    }
  }, [src])

  function handleError() {
    if (fallbackSrc && activeSrc !== fallbackSrc) {
      setActiveSrc(fallbackSrc)
      setLoaded(false)
      return
    }
    setFailed(true)
  }

  if (failed || !activeSrc) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center gap-1 p-2 text-center text-xs text-muted-foreground">
        <ImageOff className={cn('opacity-45', iconClassName)} />
        {fallbackLabel && <span className="line-clamp-2 break-all">{fallbackLabel}</span>}
      </div>
    )
  }

  return (
    <img
      ref={imgRef}
      src={activeSrc}
      alt={alt}
      loading={loading}
      decoding="async"
      onLoad={() => setLoaded(true)}
      onError={handleError}
      className={cn(
        'transition-opacity duration-200 ease-out',
        loaded ? 'opacity-100' : 'opacity-0',
        className
      )}
    />
  )
}
