import { useEffect, useRef, useState } from 'react'
import { ImageOff } from 'lucide-react'
import { cn } from '@/lib/utils'

interface FadeImageProps {
  src: string
  alt: string
  className?: string
  fallbackLabel?: string
  iconClassName?: string
  loading?: 'lazy' | 'eager'
}

/**
 * Dissolves images in smoothly once decoded instead of popping starkly from an
 * empty box, and falls back gracefully when a thumbnail cannot be loaded.
 */
export function FadeImage({
  src,
  alt,
  className,
  fallbackLabel,
  iconClassName = 'size-4',
  loading = 'lazy'
}: FadeImageProps) {
  const imgRef = useRef<HTMLImageElement>(null)
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
    if (imgRef.current?.complete && imgRef.current.naturalWidth > 0) {
      setLoaded(true)
    } else {
      setLoaded(false)
    }
  }, [src])

  if (failed || !src) {
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
      src={src}
      alt={alt}
      loading={loading}
      decoding="async"
      onLoad={() => setLoaded(true)}
      onError={() => setFailed(true)}
      className={cn(
        'transition-opacity duration-250 ease-out',
        loaded ? 'opacity-100' : 'opacity-0',
        className
      )}
    />
  )
}
