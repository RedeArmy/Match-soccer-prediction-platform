import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import {
  AdminPageHeader,
  AdminModalOverlay,
  ModalHeader,
  ModalErrorLine,
  ModalCancelButton,
  InfoRow,
  AdminPagination,
  AdminContentState,
} from '@/components/admin/shared'

// ── AdminPageHeader ───────────────────────────────────────────────────────────

describe('AdminPageHeader', () => {
  it('renders title and subtitle', () => {
    render(<AdminPageHeader title="Partidos" subtitle="10 en total" onRefresh={() => {}} isLoading={false} />)
    expect(screen.getByText('Partidos')).toBeInTheDocument()
    expect(screen.getByText('10 en total')).toBeInTheDocument()
  })

  it('calls onRefresh when Actualizar is clicked', () => {
    const handler = vi.fn()
    render(<AdminPageHeader title="T" subtitle="S" onRefresh={handler} isLoading={false} />)
    fireEvent.click(screen.getByRole('button', { name: /Actualizar/i }))
    expect(handler).toHaveBeenCalledOnce()
  })

  it('disables the refresh button while loading', () => {
    render(<AdminPageHeader title="T" subtitle="S" onRefresh={() => {}} isLoading />)
    expect(screen.getByRole('button', { name: /Actualizar/i })).toBeDisabled()
  })
})

// ── AdminModalOverlay ─────────────────────────────────────────────────────────

describe('AdminModalOverlay', () => {
  it('renders children', () => {
    render(<AdminModalOverlay onClose={() => {}}><span>contenido</span></AdminModalOverlay>)
    expect(screen.getByText('contenido')).toBeInTheDocument()
  })

  it('calls onClose when the backdrop is clicked', () => {
    const handler = vi.fn()
    const { container } = render(
      <AdminModalOverlay onClose={handler}><span>c</span></AdminModalOverlay>,
    )
    fireEvent.click(container.querySelector('[aria-hidden="true"]')!)
    expect(handler).toHaveBeenCalledOnce()
  })

  it('adds overflow-y-auto class when scrollable is true', () => {
    const { container } = render(
      <AdminModalOverlay onClose={() => {}} scrollable><span>c</span></AdminModalOverlay>,
    )
    expect(container.querySelector('.overflow-y-auto')).toBeInTheDocument()
  })

  it('does not add overflow-y-auto when scrollable is omitted', () => {
    const { container } = render(
      <AdminModalOverlay onClose={() => {}}><span>c</span></AdminModalOverlay>,
    )
    expect(container.querySelector('.overflow-y-auto')).toBeNull()
  })
})

// ── ModalHeader ───────────────────────────────────────────────────────────────

describe('ModalHeader', () => {
  it('renders the title', () => {
    render(<ModalHeader title="Aprobar retiro" onClose={() => {}} />)
    expect(screen.getByText('Aprobar retiro')).toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', () => {
    const handler = vi.fn()
    render(<ModalHeader title="T" onClose={handler} />)
    fireEvent.click(screen.getByRole('button'))
    expect(handler).toHaveBeenCalledOnce()
  })
})

// ── ModalErrorLine ────────────────────────────────────────────────────────────

describe('ModalErrorLine', () => {
  it('renders nothing when error is an empty string', () => {
    const { container } = render(<ModalErrorLine error="" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders the error message when error is non-empty', () => {
    render(<ModalErrorLine error="Algo salió mal" />)
    expect(screen.getByText('Algo salió mal')).toBeInTheDocument()
  })
})

// ── ModalCancelButton ─────────────────────────────────────────────────────────

describe('ModalCancelButton', () => {
  it('renders Cancelar text', () => {
    render(<ModalCancelButton onClose={() => {}} disabled={false} />)
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument()
  })

  it('calls onClose when clicked', () => {
    const handler = vi.fn()
    render(<ModalCancelButton onClose={handler} disabled={false} />)
    fireEvent.click(screen.getByRole('button'))
    expect(handler).toHaveBeenCalledOnce()
  })

  it('is disabled when disabled prop is true', () => {
    render(<ModalCancelButton onClose={() => {}} disabled />)
    expect(screen.getByRole('button')).toBeDisabled()
  })
})

// ── InfoRow ───────────────────────────────────────────────────────────────────

describe('InfoRow', () => {
  it('renders label and string value', () => {
    render(<InfoRow label="Monto" value="Q100.00" />)
    expect(screen.getByText('Monto')).toBeInTheDocument()
    expect(screen.getByText('Q100.00')).toBeInTheDocument()
  })

  it('renders a ReactNode value', () => {
    render(<InfoRow label="ID" value={<span data-testid="val-node">42</span>} />)
    expect(screen.getByTestId('val-node')).toBeInTheDocument()
  })
})

// ── AdminPagination ───────────────────────────────────────────────────────────

describe('AdminPagination', () => {
  const base = {
    page: 1,
    pageCount: 3,
    rangeStart: 1,
    rangeEnd: 15,
    total: 40,
    itemLabel: 'partidos',
    onPageChange: vi.fn(),
  }

  it('returns null when pageCount is 1', () => {
    const { container } = render(<AdminPagination {...base} pageCount={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders the range summary text', () => {
    render(<AdminPagination {...base} />)
    expect(screen.getByText(/1.*15 de 40 partidos/)).toBeInTheDocument()
  })

  it('disables the prev button on the first page', () => {
    render(<AdminPagination {...base} page={1} />)
    const buttons = screen.getAllByRole('button')
    expect(buttons[0]).toBeDisabled()
  })

  it('disables the next button on the last page', () => {
    render(<AdminPagination {...base} page={3} />)
    const buttons = screen.getAllByRole('button')
    expect(buttons[buttons.length - 1]).toBeDisabled()
  })

  it('calls onPageChange with the selected page number', () => {
    const handler = vi.fn()
    render(<AdminPagination {...base} onPageChange={handler} />)
    fireEvent.click(screen.getByRole('button', { name: '2' }))
    expect(handler).toHaveBeenCalledWith(2)
  })

  it('renders ellipsis separators for large page sets', () => {
    render(
      <AdminPagination
        {...base}
        page={5}
        pageCount={10}
        rangeStart={61}
        rangeEnd={75}
        total={140}
      />,
    )
    expect(screen.getAllByText('…').length).toBeGreaterThan(0)
  })
})

// ── AdminContentState ─────────────────────────────────────────────────────────

describe('AdminContentState', () => {
  const base = {
    isLoading: false,
    error: null,
    isEmpty: false,
    emptyTitle: 'Sin datos',
    emptyMessage: 'No hay nada aquí.',
    errorMessage: 'Error al cargar.',
  }

  it('renders the LoadingState skeleton when isLoading is true', () => {
    const { container } = render(
      <AdminContentState {...base} isLoading>
        <span>data</span>
      </AdminContentState>,
    )
    expect(container.querySelectorAll('.h-16').length).toBeGreaterThan(0)
    expect(screen.queryByText('data')).toBeNull()
  })

  it('renders the error message when error is truthy', () => {
    render(
      <AdminContentState {...base} error={new Error('boom')}>
        <span>data</span>
      </AdminContentState>,
    )
    expect(screen.getByText('Error al cargar.')).toBeInTheDocument()
    expect(screen.queryByText('data')).toBeNull()
  })

  it('renders the empty state when isEmpty is true', () => {
    render(
      <AdminContentState {...base} isEmpty>
        <span>data</span>
      </AdminContentState>,
    )
    expect(screen.getByText('Sin datos')).toBeInTheDocument()
    expect(screen.getByText('No hay nada aquí.')).toBeInTheDocument()
    expect(screen.queryByText('data')).toBeNull()
  })

  it('renders children when not loading, no error, and not empty', () => {
    render(
      <AdminContentState {...base}>
        <span>table data</span>
      </AdminContentState>,
    )
    expect(screen.getByText('table data')).toBeInTheDocument()
  })
})
