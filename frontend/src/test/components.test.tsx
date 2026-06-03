import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { EmptyState }     from '@/components/shared/EmptyState'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'
import { StatusBadge }    from '@/components/shared/StatusBadge'

// ── EmptyState ────────────────────────────────────────────────────────────────

describe('EmptyState', () => {
  it('renders title', () => {
    render(<EmptyState title="No hay datos" />)
    expect(screen.getByText('No hay datos')).toBeInTheDocument()
  })

  it('renders optional description', () => {
    render(<EmptyState title="T" description="Sin resultados por ahora" />)
    expect(screen.getByText('Sin resultados por ahora')).toBeInTheDocument()
  })

  it('renders optional action', () => {
    render(<EmptyState title="T" action={<button>Reintentar</button>} />)
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
  })

  it('renders optional icon', () => {
    render(<EmptyState title="T" icon={<span data-testid="icon" />} />)
    expect(screen.getByTestId('icon')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    const { container } = render(<EmptyState title="T" className="my-class" />)
    expect(container.firstElementChild?.className).toContain('my-class')
  })
})

// ── LoadingState ──────────────────────────────────────────────────────────────

describe('LoadingState', () => {
  it('renders default 3 skeleton rows', () => {
    const { container } = render(<LoadingState />)
    expect(container.querySelectorAll('.h-16')).toHaveLength(3)
  })

  it('renders N rows when rows prop is given', () => {
    const { container } = render(<LoadingState rows={5} />)
    expect(container.querySelectorAll('.h-16')).toHaveLength(5)
  })

  it('applies custom className', () => {
    const { container } = render(<LoadingState className="my-loader" />)
    expect(container.firstElementChild?.className).toContain('my-loader')
  })
})

describe('LoadingSpinner', () => {
  it('renders svg with aria-label', () => {
    render(<LoadingSpinner />)
    expect(screen.getByLabelText('Cargando')).toBeInTheDocument()
  })

  it('applies custom size', () => {
    render(<LoadingSpinner size={32} />)
    const svg = screen.getByLabelText('Cargando')
    expect(svg.getAttribute('width')).toBe('32')
    expect(svg.getAttribute('height')).toBe('32')
  })
})

// ── StatusBadge ───────────────────────────────────────────────────────────────

describe('StatusBadge', () => {
  it('renders known status in Spanish', () => {
    render(<StatusBadge status="active" />)
    expect(screen.getByText('Activo')).toBeInTheDocument()
  })

  it('renders pending label', () => {
    render(<StatusBadge status="pending" />)
    expect(screen.getByText('Pendiente')).toBeInTheDocument()
  })

  it('renders approved label', () => {
    render(<StatusBadge status="approved" />)
    expect(screen.getByText('Aprobado')).toBeInTheDocument()
  })

  it('renders rejected label', () => {
    render(<StatusBadge status="rejected" />)
    expect(screen.getByText('Rechazado')).toBeInTheDocument()
  })

  it('falls back to raw status for unknown values', () => {
    render(<StatusBadge status="foobar" />)
    expect(screen.getByText('foobar')).toBeInTheDocument()
  })

  it('applies sm size class', () => {
    const { container } = render(<StatusBadge status="active" size="sm" />)
    expect(container.firstElementChild?.className).toContain('text-[10px]')
  })

  it('applies md size class by default', () => {
    const { container } = render(<StatusBadge status="active" />)
    expect(container.firstElementChild?.className).toContain('text-xs')
  })

  it('applies custom className', () => {
    const { container } = render(<StatusBadge status="active" className="extra" />)
    expect(container.firstElementChild?.className).toContain('extra')
  })
})
