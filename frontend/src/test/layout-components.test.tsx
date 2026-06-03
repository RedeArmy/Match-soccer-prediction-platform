import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import React from 'react'

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock('next/link', () => ({
  default: ({ children, href, onClick, className }: {
    children: React.ReactNode
    href: string
    onClick?: React.MouseEventHandler
    className?: string
  }) => <a href={href} onClick={onClick} className={className}>{children}</a>,
}))

const mockUsePathname = vi.fn().mockReturnValue('/')

vi.mock('next/navigation', () => ({
  usePathname: () => mockUsePathname(),
}))

vi.mock('@clerk/nextjs', () => ({
  useAuth:    vi.fn().mockReturnValue({ getToken: vi.fn().mockResolvedValue('tok') }),
  useClerk:   vi.fn().mockReturnValue({ signOut: vi.fn() }),
  SignedIn:   ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SignedOut:  ({ children }: { children: React.ReactNode }) => <>{children}</>,
  UserButton: () => null,
}))

vi.mock('@/hooks/useExchangeRate', () => ({
  useExchangeRate: vi.fn(),
}))

vi.mock('@/hooks/useBalance', () => ({
  useBalance: vi.fn(),
}))

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useExchangeRate } from '@/hooks/useExchangeRate'
import { useBalance }      from '@/hooks/useBalance'
import { Footer }          from '@/components/layout/Footer'
import { Header }          from '@/components/layout/Header'
import { AdminSidebar }    from '@/components/layout/AdminSidebar'
import { MobileNav }       from '@/components/layout/MobileNav'
import { BalanceCard }     from '@/components/balance/BalanceCard'
import { ExchangeRateTicker } from '@/components/exchange/RateTicker'

// ── Footer ────────────────────────────────────────────────────────────────────

describe('Footer', () => {
  it('renders "QM" brand text', () => {
    render(<Footer />)
    expect(screen.getByText('QM')).toBeInTheDocument()
  })

  it('renders "Torneos" link with href=/tournaments', () => {
    render(<Footer />)
    const link = screen.getByRole('link', { name: 'Torneos' })
    expect(link).toHaveAttribute('href', '/tournaments')
  })
})

// ── Header ────────────────────────────────────────────────────────────────────

describe('Header', () => {
  beforeEach(() => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: false } as never)
  })

  it('renders logo link', () => {
    render(<Header />)
    // The logo link wraps "QM" text
    const logo = screen.getAllByText('QM')[0]
    expect(logo).toBeInTheDocument()
  })

  it('shows exchange rate ticker when rate data is provided', () => {
    vi.mocked(useExchangeRate).mockReturnValue({
      data: { sell_rate: '7.85', buy_rate: '7.72', effective_at: '2026-01-01T00:00:00Z', stale: false },
      isLoading: false,
    } as never)
    render(<Header />)
    // The ticker shows the sell_rate formatted
    expect(screen.getByText(/7\.8500/)).toBeInTheDocument()
  })

  it('does not show ticker when rate is undefined', () => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: false } as never)
    render(<Header />)
    expect(screen.queryByText(/7\./)).toBeNull()
  })

  it('renders mobile menu button with aria-label="Abrir menú"', () => {
    render(<Header />)
    expect(screen.getByRole('button', { name: 'Abrir menú' })).toBeInTheDocument()
  })
})

// ── AdminSidebar ──────────────────────────────────────────────────────────────

describe('AdminSidebar', () => {
  beforeEach(() => {
    mockUsePathname.mockReturnValue('/')
  })

  it('renders all 8 nav items', () => {
    render(<AdminSidebar />)
    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.getByText('Usuarios')).toBeInTheDocument()
    expect(screen.getByText('KYC Queue')).toBeInTheDocument()
    expect(screen.getByText('Torneos')).toBeInTheDocument()
    expect(screen.getByText('Retiros')).toBeInTheDocument()
    expect(screen.getByText('Tipo de cambio')).toBeInTheDocument()
    expect(screen.getByText('Observabilidad')).toBeInTheDocument()
    expect(screen.getByText('Parámetros')).toBeInTheDocument()
  })

  it('applies active class when pathname matches nav item', () => {
    mockUsePathname.mockReturnValue('/admin/dashboard')
    render(<AdminSidebar />)
    const dashboardLink = screen.getByRole('link', { name: /Dashboard/ })
    expect(dashboardLink.className).toContain('bg-blue-800')
  })
})

// ── MobileNav ─────────────────────────────────────────────────────────────────

describe('MobileNav', () => {
  it('drawer has translate-x-full class when open=false', () => {
    const { container } = render(<MobileNav open={false} onClose={vi.fn()} />)
    const aside = container.querySelector('aside')
    expect(aside?.className).toContain('translate-x-full')
  })

  it('drawer has translate-x-0 class when open=true', () => {
    const { container } = render(<MobileNav open={true} onClose={vi.fn()} />)
    const aside = container.querySelector('aside')
    expect(aside?.className).toContain('translate-x-0')
  })

  it('clicking close button calls onClose', () => {
    const onClose = vi.fn()
    render(<MobileNav open={true} onClose={onClose} />)
    // The close button has an X icon; find it by its parent button
    const buttons = screen.getAllByRole('button')
    // The first button in the drawer header is the close button
    fireEvent.click(buttons[0])
    expect(onClose).toHaveBeenCalled()
  })

  it('clicking backdrop calls onClose', () => {
    const onClose = vi.fn()
    const { container } = render(<MobileNav open={true} onClose={onClose} />)
    const backdrop = container.querySelector('.fixed.inset-0')
    fireEvent.click(backdrop!)
    expect(onClose).toHaveBeenCalled()
  })
})

// ── BalanceCard ───────────────────────────────────────────────────────────────

describe('BalanceCard', () => {
  beforeEach(() => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: false } as never)
  })

  it('shows loading state when isLoading=true', () => {
    vi.mocked(useBalance).mockReturnValue({ isLoading: true, data: undefined } as never)
    const { container } = render(<BalanceCard />)
    // LoadingState renders skeleton rows with h-16 class
    expect(container.querySelector('.h-16')).toBeInTheDocument()
  })

  it('shows available balance in GTQ format', () => {
    vi.mocked(useBalance).mockReturnValue({
      data: { available_cents: 50000, reserved_cents: 0, pending_cents: 0 },
      isLoading: false,
    } as never)
    render(<BalanceCard />)
    // 50000 cents = Q500.00; Intl formats as "Q500.00" or similar
    expect(screen.getByText(/500/)).toBeInTheDocument()
  })

  it('toggle button switches currency display label', () => {
    vi.mocked(useBalance).mockReturnValue({
      data: { available_cents: 50000, reserved_cents: 0, pending_cents: 0 },
      isLoading: false,
    } as never)
    render(<BalanceCard />)
    const toggleBtn = screen.getByRole('button', { name: /→ USD/ })
    expect(toggleBtn).toBeInTheDocument()
    fireEvent.click(toggleBtn)
    expect(screen.getByRole('button', { name: /→ GTQ/ })).toBeInTheDocument()
  })
})

// ── ExchangeRateTicker ────────────────────────────────────────────────────────

describe('ExchangeRateTicker', () => {
  it('returns null when loading', () => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: true } as never)
    const { container } = render(<ExchangeRateTicker />)
    expect(container.firstChild).toBeNull()
  })

  it('renders sell_rate and "Tipo de cambio" text when data is available', () => {
    vi.mocked(useExchangeRate).mockReturnValue({
      data: { buy_rate: '7.72', sell_rate: '7.8500', effective_at: '2026-01-01T00:00:00Z', stale: false },
      isLoading: false,
    } as never)
    render(<ExchangeRateTicker />)
    expect(screen.getByText(/Tipo de cambio/)).toBeInTheDocument()
    expect(screen.getByText(/7\.8500/)).toBeInTheDocument()
  })

  it('shows stale warning when rate.stale=true', () => {
    vi.mocked(useExchangeRate).mockReturnValue({
      data: { buy_rate: '7.72', sell_rate: '7.80', effective_at: '2026-01-01T00:00:00Z', stale: true },
      isLoading: false,
    } as never)
    render(<ExchangeRateTicker />)
    expect(screen.getByText(/desactualizado/)).toBeInTheDocument()
  })
})
