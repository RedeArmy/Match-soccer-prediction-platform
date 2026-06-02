import { ImageIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ImagePlaceholderProps {
  aspectRatio?: string
  label?:       string
  dataSrc?:     string
  className?:   string
  rounded?:     boolean
}

export function ImagePlaceholder({
  aspectRatio = '16/9',
  label,
  dataSrc,
  className,
  rounded = true,
}: ImagePlaceholderProps) {
  return (
    <div
      className={cn(
        'relative overflow-hidden bg-blue-800 border border-blue-600/50',
        'flex flex-col items-center justify-center gap-2',
        rounded && 'rounded-xl',
        className,
      )}
      style={{ aspectRatio }}
      data-future-src={dataSrc}
      role="img"
      aria-label={label ?? 'Image placeholder'}
    >
      <ImageIcon className="w-8 h-8 text-blue-400/60" />
      {label && (
        <span className="text-xs text-blue-400/80 text-center px-4 leading-tight">
          {label}
        </span>
      )}
      {dataSrc && (
        <span className="text-[10px] text-blue-600 font-mono mt-1 px-2 text-center break-all">
          {dataSrc}
        </span>
      )}
    </div>
  )
}
