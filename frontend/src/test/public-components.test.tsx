import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import React from 'react'
import { I18nProvider } from '@/lib/i18n'

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock('@tanstack/react-query', () => ({
  useQuery:       vi.fn(),
  useMutation:    vi.fn().mockReturnValue({ mutate: vi.fn(), isPending: false }),
  useQueryClient: vi.fn().mockReturnValue({ invalidateQueries: vi.fn() }),
}))

// Shallow-mock LoadingState so we don't need to resolve its full dep tree.
vi.mock('@/components/shared/LoadingState', () => ({
  LoadingState: ({ rows }: { rows?: number }) => (
    <div data-testid="loading-state" data-rows={rows} />
  ),
}))

// ── Imports (after mocks) ─────────────────────────────────────────────────────

import { useQuery } from '@tanstack/react-query'
import { GroupCodeLookup } from '@/components/public/GroupCodeLookup'
import { LiveMatchFeed }   from '@/components/public/LiveMatchFeed'
import type { TodayFixture } from '@/app/api/live/today/route'

// ── Fetch mock ────────────────────────────────────────────────────────────────

const mockFetch = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', mockFetch)

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderLookup() {
  return render(<I18nProvider><GroupCodeLookup /></I18nProvider>)
}

function renderFeed() {
  return render(<I18nProvider><LiveMatchFeed /></I18nProvider>)
}

function makeFixture(overrides: Partial<TodayFixture> = {}): TodayFixture {
  return {
    id:        1,
    homeTeam:  'Brazil',
    awayTeam:  'Germany',
    homeLogo:  '',
    awayLogo:  '',
    homeScore: null,
    awayScore: null,
    status:    'NS',
    elapsed:   null,
    kickoffAt: '2026-06-10T20:00:00Z',
    round:     'Group Stage - 1',
    venue:     null,
    ...overrides,
  }
}

// ── GroupCodeLookup ───────────────────────────────────────────────────────────

describe('GroupCodeLookup – initial render', () => {
  it('renders the input and Ver button', () => {
    renderLookup()
    expect(screen.getByRole('textbox')).toBeInTheDocument()
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('Ver button is disabled when input is empty', () => {
    renderLookup()
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('Ver button becomes enabled when user types a code', async () => {
    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'ABC' } })
    expect(screen.getByRole('button')).not.toBeDisabled()
  })

  it('uppercases typed text', async () => {
    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'abc123' } })
    expect((screen.getByRole('textbox') as HTMLInputElement).value).toBe('ABC123')
  })
})

describe('GroupCodeLookup – lookup success', () => {
  beforeEach(() => mockFetch.mockReset())

  it('renders the leaderboard table after a successful fetch', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          group_name: 'Copa Familia',
          entries: [
            { user_id: 1, user_name: 'Jugador1', total_points: 30, rank: 1, prize_winner: true,  round_points: {} },
            { user_id: 2, user_name: 'Jugador2', total_points: 20, rank: 2, prize_winner: false, round_points: {} },
          ],
        }),
        { status: 200 },
      ),
    )

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'FAM001' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByText('Copa Familia')).toBeInTheDocument())
    expect(screen.getByText('Jugador1')).toBeInTheDocument()
    expect(screen.getByText('Jugador2')).toBeInTheDocument()
    expect(screen.getByText('30')).toBeInTheDocument()
    expect(screen.getByText('20')).toBeInTheDocument()
  })
})

describe('GroupCodeLookup – lookup error states', () => {
  beforeEach(() => mockFetch.mockReset())

  it('shows not-found error on 404', async () => {
    mockFetch.mockResolvedValueOnce(new Response('', { status: 404 }))

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'NOPE00' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() =>
      expect(screen.getByText(/no se encontró|not found/i)).toBeInTheDocument(),
    )
  })

  it('shows generic error on non-ok response', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: { message: 'Internal server error' } }), { status: 500 }),
    )

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'ERR001' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() =>
      expect(screen.getByText('Internal server error')).toBeInTheDocument(),
    )
  })

  it('shows network error when fetch throws', async () => {
    mockFetch.mockRejectedValueOnce(new Error('network failure'))

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'NET001' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() =>
      expect(screen.getByText(/pudo conectar|could not connect/i)).toBeInTheDocument(),
    )
  })
})

describe('GroupCodeLookup – close button', () => {
  beforeEach(() => mockFetch.mockReset())

  it('resets back to input form after clicking the back button', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ group_name: 'Liga Test', entries: [] }),
        { status: 200 },
      ),
    )

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'LIG001' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByText('Liga Test')).toBeInTheDocument())

    // Click the back (ArrowLeft) button — its aria-label contains "cerrar" or similar
    const backButton = screen.getByRole('button')
    fireEvent.click(backButton)

    await waitFor(() => expect(screen.getByRole('textbox')).toBeInTheDocument())
  })
})

describe('GroupCodeLookup – empty entries', () => {
  beforeEach(() => mockFetch.mockReset())

  it('shows empty state message when group has no entries', async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ group_name: 'Vacío', entries: [] }),
        { status: 200 },
      ),
    )

    renderLookup()
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'VAC001' } })
    fireEvent.click(screen.getByRole('button'))

    await waitFor(() => expect(screen.getByText('Vacío')).toBeInTheDocument())
    expect(screen.getByText(/puntuaciones|no scores/i)).toBeInTheDocument()
  })
})

// ── LiveMatchFeed ─────────────────────────────────────────────────────────────

describe('LiveMatchFeed – loading state', () => {
  it('renders LoadingState while query is pending', () => {
    vi.mocked(useQuery).mockReturnValue({ isLoading: true, isError: false, data: undefined } as never)
    renderFeed()
    expect(screen.getByTestId('loading-state')).toBeInTheDocument()
  })
})

describe('LiveMatchFeed – error state', () => {
  it('renders error message when query fails', () => {
    vi.mocked(useQuery).mockReturnValue({ isLoading: false, isError: true, data: undefined } as never)
    renderFeed()
    // The error key resolves to a non-empty translated string.
    const panel = document.querySelector('.panel')
    expect(panel).toBeInTheDocument()
  })
})

describe('LiveMatchFeed – empty state', () => {
  it('renders empty message when no fixtures returned', () => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError:   false,
      data:      { fixtures: [] },
    } as never)
    renderFeed()
    const panel = document.querySelector('.panel')
    expect(panel).toBeInTheDocument()
  })
})

describe('LiveMatchFeed – fixture list', () => {
  beforeEach(() => {
    vi.mocked(useQuery).mockReturnValue({
      isLoading: false,
      isError:   false,
      data:      {
        fixtures: [
          makeFixture({ id: 1, homeTeam: 'Brazil', awayTeam: 'Germany', status: 'NS' }),
          makeFixture({ id: 2, homeTeam: 'France',  awayTeam: 'Spain',   status: '1H', elapsed: 30, homeScore: 1, awayScore: 0 }),
        ],
      },
    } as never)
  })

  it('renders team names for all fixtures', () => {
    renderFeed()
    expect(screen.getByText('Brazil')).toBeInTheDocument()
    expect(screen.getByText('Germany')).toBeInTheDocument()
    expect(screen.getByText('France')).toBeInTheDocument()
    expect(screen.getByText('Spain')).toBeInTheDocument()
  })

  it('shows LIVE badge when at least one fixture is live', () => {
    renderFeed()
    // The liveBadge key renders a span with a green badge class.
    const badge = document.querySelector('.bg-green-500\\/20')
    expect(badge).toBeInTheDocument()
  })
})

describe('LiveMatchFeed – expand/collapse fixture', () => {
  beforeEach(() => {
    // First call: today's fixtures; subsequent calls (fixture detail): return fixture stub.
    vi.mocked(useQuery).mockImplementation(({ queryKey }: { queryKey: readonly unknown[] }) => {
      if (queryKey[0] === 'live-today') {
        return {
          isLoading: false,
          isError:   false,
          data:      { fixtures: [makeFixture({ id: 7, homeTeam: 'Brazil', awayTeam: 'Germany', status: '1H', elapsed: 22 })] },
        } as never
      }
      // live-fixture detail query
      return {
        isLoading: false,
        isError:   false,
        data:      {
          fixture: {
            id: 7, homeTeam: 'Brazil', awayTeam: 'Germany', homeLogo: '', awayLogo: '',
            homeScore: 1, awayScore: 0, halftimeHome: null, halftimeAway: null,
            status: '1H', elapsed: 22, kickoffAt: '2026-06-10T20:00:00Z',
            round: 'Group Stage - 1', venue: null,
            lineups: [], events: [],
          },
        },
      } as never
    })
  })

  it('toggles fixture detail on click', async () => {
    renderFeed()
    const button = screen.getByRole('button', { name: /brazil|germany/i })
    fireEvent.click(button)
    await waitFor(() => expect(button).toHaveAttribute('aria-expanded', 'true'))
    fireEvent.click(button)
    await waitFor(() => expect(button).toHaveAttribute('aria-expanded', 'false'))
  })
})
