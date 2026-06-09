import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render as rtlRender, screen, fireEvent, waitFor } from '@testing-library/react'
import React from 'react'
import { I18nProvider, useI18n } from '@/lib/i18n'

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
import { LanguageSwitcher }from '@/components/layout/LanguageSwitcher'
import { BalanceCard }     from '@/components/balance/BalanceCard'
import { ExchangeRateTicker } from '@/components/exchange/RateTicker'
import { FeaturedPoolsSection } from '@/components/groups/FeaturedPoolsSection'

function render(ui: React.ReactElement) {
  return rtlRender(<I18nProvider>{ui}</I18nProvider>)
}

// ── Footer ────────────────────────────────────────────────────────────────────

describe('Footer', () => {
  it('renders "Q26" brand text', () => {
    render(<Footer />)
    expect(screen.getByText('K26')).toBeInTheDocument()
  })

  it('renders "Fan Fest" link with href=/tournaments', () => {
    render(<Footer />)
    const link = screen.getByRole('link', { name: 'Fan Fest' })
    expect(link).toHaveAttribute('href', '/tournaments')
  })
})

// ── Header ────────────────────────────────────────────────────────────────────

describe('Header', () => {
  it('renders logo link', () => {
    render(<Header />)
    const logo = screen.getAllByText('K26')[0]
    expect(logo).toBeInTheDocument()
  })

  it('renders mobile menu button with aria-label="Abrir menu"', () => {
    render(<Header />)
    expect(screen.getByRole('button', { name: 'Abrir menu' })).toBeInTheDocument()
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

  it('shows no manual currency toggle — currency follows locale', () => {
    vi.mocked(useBalance).mockReturnValue({
      data: { available_cents: 50000, reserved_cents: 0, pending_cents: 0 },
      isLoading: false,
    } as never)
    render(<BalanceCard />)
    // Currency is now automatic (locale-driven), so no toggle buttons should exist
    expect(screen.queryByRole('button', { name: /-> USD/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /-> GTQ/ })).toBeNull()
    // Default locale is Spanish → GTQ; Q500.00 for 50000 cents
    expect(screen.getByText(/500/)).toBeInTheDocument()
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

// ── BalanceCard reserved/pending branches ─────────────────────────────────────

describe('BalanceCard – non-zero reserved and pending', () => {
  beforeEach(() => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: false } as never)
  })

  it('shows reserved amount when reserved_cents > 0', () => {
    vi.mocked(useBalance).mockReturnValue({
      data: { available_cents: 50000, reserved_cents: 5000, pending_cents: 0 },
      isLoading: false,
    } as never)
    render(<BalanceCard />)
    expect(screen.getByText(/Reservado/)).toBeInTheDocument()
  })

  it('shows pending amount when pending_cents > 0', () => {
    vi.mocked(useBalance).mockReturnValue({
      data: { available_cents: 50000, reserved_cents: 0, pending_cents: 2500 },
      isLoading: false,
    } as never)
    render(<BalanceCard />)
    expect(screen.getByText(/Pendiente/)).toBeInTheDocument()
  })
})

// ── LanguageSwitcher ──────────────────────────────────────────────────────────

describe('LanguageSwitcher', () => {
  it('renders es and en buttons', () => {
    render(<LanguageSwitcher />)
    expect(screen.getByRole('button', { name: 'es' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'en' })).toBeInTheDocument()
  })

  it('calling setLocale persists locale to localStorage', () => {
    render(<LanguageSwitcher />)
    fireEvent.click(screen.getByRole('button', { name: 'en' }))
    expect(localStorage.getItem('quiniela-locale')).toBe('en')
    localStorage.removeItem('quiniela-locale')
  })
})

// ── I18nProvider locale restoration ──────────────────────────────────────────

describe('I18nProvider', () => {
  it('restores locale from localStorage on mount', async () => {
    localStorage.setItem('quiniela-locale', 'en')

    function TestLocale() {
      const { locale } = useI18n()
      return <span data-testid="locale">{locale}</span>
    }

    render(<TestLocale />)
    await waitFor(() => {
      expect(screen.getByTestId('locale').textContent).toBe('en')
    })

    localStorage.removeItem('quiniela-locale')
  })
})

// ── FeaturedPoolsSection ──────────────────────────────────────────────────────

describe('FeaturedPoolsSection', () => {
  beforeEach(() => {
    vi.mocked(useExchangeRate).mockReturnValue({ data: undefined, isLoading: false } as never)
  })

  it('renders all three featured groups', () => {
    render(<FeaturedPoolsSection />)
    expect(screen.getByText('World Cup 2026 Global Pool')).toBeInTheDocument()
    expect(screen.getByText('Americas Knockout Challenge')).toBeInTheDocument()
    expect(screen.getByText('Finals Elite Quiniela')).toBeInTheDocument()
  })

  it('shows Enter button for open group and Details for upcoming', () => {
    render(<FeaturedPoolsSection />)
    expect(screen.getByText('Entrar')).toBeInTheDocument()
    expect(screen.getAllByText('Ver detalles').length).toBeGreaterThan(0)
  })
})
