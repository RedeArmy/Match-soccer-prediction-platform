import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { EmptyState }     from '@/components/shared/EmptyState'
import { LoadingState, LoadingSpinner } from '@/components/shared/LoadingState'
import { StatusBadge }    from '@/components/shared/StatusBadge'
import { FormFeedback }   from '@/components/shared/FormFeedback'
import { FileUploadField } from '@/components/shared/FileUploadField'
import { SubmitButton }   from '@/components/shared/SubmitButton'

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

// ── FormFeedback ──────────────────────────────────────────────────────────────

describe('FormFeedback', () => {
  it('renders nothing when both error and success are absent', () => {
    const { container } = render(<FormFeedback />)
    expect(container.firstChild).toBeNull()
  })

  it('renders error message', () => {
    render(<FormFeedback error="Something went wrong" />)
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })

  it('renders success message', () => {
    render(<FormFeedback success="Saved successfully" />)
    expect(screen.getByText('Saved successfully')).toBeInTheDocument()
  })

  it('renders both error and success when both are provided', () => {
    render(<FormFeedback error="Err" success="Ok" />)
    expect(screen.getByText('Err')).toBeInTheDocument()
    expect(screen.getByText('Ok')).toBeInTheDocument()
  })

  it('applies text-center class when center prop is true', () => {
    const { container } = render(<FormFeedback error="Err" center />)
    expect(container.querySelector('p')?.className).toContain('text-center')
  })

  it('does not apply text-center class by default', () => {
    const { container } = render(<FormFeedback error="Err" />)
    expect(container.querySelector('p')?.className).not.toContain('text-center')
  })
})

// ── FileUploadField ───────────────────────────────────────────────────────────

describe('FileUploadField', () => {
  it('renders label text', () => {
    render(<FileUploadField label="Adjuntar archivo" onChange={() => {}} />)
    expect(screen.getByText('Adjuntar archivo')).toBeInTheDocument()
  })

  it('renders fileName instead of label when fileName is provided', () => {
    render(<FileUploadField label="Adjuntar" fileName="foto.jpg" onChange={() => {}} />)
    expect(screen.getByText('foto.jpg')).toBeInTheDocument()
    expect(screen.queryByText('Adjuntar')).toBeNull()
  })

  it('renders a hidden file input', () => {
    const { container } = render(<FileUploadField label="Upload" onChange={() => {}} />)
    const input = container.querySelector('input[type="file"]')
    expect(input).toBeInTheDocument()
    expect(input?.className).toContain('sr-only')
  })

  it('uses default accept value', () => {
    const { container } = render(<FileUploadField label="Upload" onChange={() => {}} />)
    const input = container.querySelector('input[type="file"]')
    expect(input?.getAttribute('accept')).toContain('image/jpeg')
  })

  it('calls onChange when a file is selected', () => {
    const handler = vi.fn()
    const { container } = render(<FileUploadField label="Upload" onChange={handler} />)
    const input = container.querySelector('input[type="file"]')!
    fireEvent.change(input, { target: { files: [] } })
    expect(handler).toHaveBeenCalledOnce()
  })
})

// ── SubmitButton ──────────────────────────────────────────────────────────────

describe('SubmitButton', () => {
  it('renders children when not pending', () => {
    render(<SubmitButton isPending={false}>Guardar</SubmitButton>)
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeInTheDocument()
  })

  it('renders spinner and hides children when isPending is true', () => {
    render(<SubmitButton isPending>Guardar</SubmitButton>)
    expect(screen.queryByText('Guardar')).toBeNull()
    expect(screen.getByLabelText('Cargando')).toBeInTheDocument()
  })

  it('is disabled when isPending is true', () => {
    render(<SubmitButton isPending>Enviar</SubmitButton>)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('is disabled when disabled prop is true', () => {
    render(<SubmitButton isPending={false} disabled>Enviar</SubmitButton>)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('is enabled when neither isPending nor disabled', () => {
    render(<SubmitButton isPending={false}>Enviar</SubmitButton>)
    expect(screen.getByRole('button')).not.toBeDisabled()
  })

  it('calls onClick when clicked', () => {
    const handler = vi.fn()
    render(<SubmitButton isPending={false} onClick={handler}>Click</SubmitButton>)
    fireEvent.click(screen.getByRole('button'))
    expect(handler).toHaveBeenCalledOnce()
  })
})
