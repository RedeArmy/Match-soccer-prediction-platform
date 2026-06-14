'use client'

import React, { useEffect, useRef, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'

interface HorizontalCarouselProps {
  readonly children: React.ReactNode
  readonly itemWidth?: string
  readonly gap?: string
  readonly scrollAmount?: number
  readonly ariaLabelLeft?: string
  readonly ariaLabelRight?: string
}

export function HorizontalCarousel({
  children,
  itemWidth = 'w-72',
  gap = 'gap-4',
  scrollAmount = 300,
  ariaLabelLeft,
  ariaLabelRight,
}: HorizontalCarouselProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [canLeft, setCanLeft] = useState(false)
  const [canRight, setCanRight] = useState(false)

  function scroll(dir: 'left' | 'right') {
    trackRef.current?.scrollBy({ left: dir === 'right' ? scrollAmount : -scrollAmount, behavior: 'smooth' })
  }

  useEffect(() => {
    const el = trackRef.current
    if (!el) return
    function update() {
      setCanLeft(el!.scrollLeft > 4)
      setCanRight(el!.scrollLeft + el!.clientWidth < el!.scrollWidth - 4)
    }
    update()
    el.addEventListener('scroll', update, { passive: true })
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => { el.removeEventListener('scroll', update); ro.disconnect() }
  }, [children])

  return (
    <div className="relative">
      {canLeft && (
        <button
          type="button"
          onClick={() => scroll('left')}
          className="absolute -left-3 top-1/2 z-10 -translate-y-1/2 rounded-full border border-white/10 bg-[#0b1929] p-1.5 text-text-muted shadow-lg transition-colors hover:text-white"
          aria-label={ariaLabelLeft}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
      )}

      <div
        ref={trackRef}
        className={`flex ${gap} overflow-x-auto pb-2 scrollbar-hide`}
        style={{ scrollSnapType: 'x mandatory' }}
      >
        {React.Children.map(children, (child) => (
          <div className={`${itemWidth} shrink-0`} style={{ scrollSnapAlign: 'start' }}>
            {child}
          </div>
        ))}
      </div>

      {canRight && (
        <button
          type="button"
          onClick={() => scroll('right')}
          className="absolute -right-3 top-1/2 z-10 -translate-y-1/2 rounded-full border border-white/10 bg-[#0b1929] p-1.5 text-text-muted shadow-lg transition-colors hover:text-white"
          aria-label={ariaLabelRight}
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      )}
    </div>
  )
}
