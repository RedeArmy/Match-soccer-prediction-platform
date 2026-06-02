import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ImagePlaceholder } from '@/components/shared/ImagePlaceholder'

describe('ImagePlaceholder', () => {
  it('renders with correct aspect ratio', () => {
    const { container } = render(<ImagePlaceholder aspectRatio="16/9" />)
    const el = container.firstElementChild as HTMLElement
    expect(el.style.aspectRatio).toBe('16/9')
  })

  it('shows label text', () => {
    render(<ImagePlaceholder label="Hero banner" />)
    expect(screen.getByText('Hero banner')).toBeInTheDocument()
  })

  it('shows dataSrc path', () => {
    render(<ImagePlaceholder dataSrc="/images/hero.jpg" />)
    expect(screen.getByText('/images/hero.jpg')).toBeInTheDocument()
  })

  it('stores dataSrc as data attribute', () => {
    const { container } = render(<ImagePlaceholder dataSrc="/images/hero.jpg" />)
    const el = container.firstElementChild as HTMLElement
    expect(el.dataset.futureSrc).toBe('/images/hero.jpg')
  })

  it('applies rounded class by default', () => {
    const { container } = render(<ImagePlaceholder />)
    expect(container.firstElementChild?.className).toContain('rounded-xl')
  })

  it('omits rounded class when rounded=false', () => {
    const { container } = render(<ImagePlaceholder rounded={false} />)
    expect(container.firstElementChild?.className).not.toContain('rounded-xl')
  })
})
