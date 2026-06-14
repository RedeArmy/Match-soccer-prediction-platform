import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@/lib/i18n'
import { PredictionPanel } from '@/components/predictions/PredictionPanel'
import { api } from '@/lib/api'

vi.mock('@clerk/nextjs', () => ({
  useAuth: vi.fn().mockReturnValue({ getToken: vi.fn().mockResolvedValue('tok') }),
}))

// Stub the system-clock endpoint so PredictionPanel's systemClockQuery resolves.
vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
  ok: true,
  json: () => Promise.resolve({ now: new Date().toISOString() }),
} as unknown as Response))

vi.mock('@/lib/api', () => ({
  api: {
    getMatches: vi.fn(),
    getMyPredictions: vi.fn(),
    submitPrediction: vi.fn(),
    updatePrediction: vi.fn(),
  },
}))

const futureKickoff = '2099-06-11T18:00:00Z'
const pastKickoff = '2026-01-01T18:00:00Z'

const scheduledMatch = {
  id: 10,
  home_team: 'Canada',
  away_team: 'Mexico',
  home_score: null,
  away_score: null,
  status: 'scheduled',
  kickoff_at: futureKickoff,
  stadium: { name: 'Toronto Stadium' },
  phase: 'group_stage',
  group_label: 'A',
}

const lockedMatch = {
  ...scheduledMatch,
  id: 11,
  home_team: 'USA',
  away_team: 'Guatemala',
  status: 'in_progress',
  kickoff_at: pastKickoff,
  stadium: null,
  phase: null,
  group_label: 'B',
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <PredictionPanel />
      </I18nProvider>
    </QueryClientProvider>,
  )
}

describe('PredictionPanel', () => {
  beforeEach(() => {
    vi.mocked(api.getMatches).mockReset()
    vi.mocked(api.getMyPredictions).mockReset()
    vi.mocked(api.submitPrediction).mockReset()
    vi.mocked(api.updatePrediction).mockReset()
    vi.mocked(api.submitPrediction).mockResolvedValue({
      id: 1,
      match_id: scheduledMatch.id,
      home_score: 2,
      away_score: 1,
      points: null,
      scored_at: null,
      created_at: '',
    })
    vi.mocked(api.updatePrediction).mockResolvedValue({
      id: 99,
      match_id: scheduledMatch.id,
      home_score: 3,
      away_score: 1,
      points: 5,
      scored_at: null,
      created_at: '',
    })
  })

  it('renders the empty state when no matches exist', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([])
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()

    // by-group view (default) with no matches shows the group-level empty state
    expect(await screen.findByText('Sin partidos en este grupo')).toBeInTheDocument()
    expect(screen.getByText(/Este grupo no tiene partidos/)).toBeInTheDocument()
  })

  it('submits a new prediction for a scheduled match', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()

    expect(await screen.findByText('Canadá')).toBeInTheDocument()
    const inputs = screen.getAllByRole('spinbutton')
    fireEvent.change(inputs[0], { target: { value: '2' } })
    fireEvent.change(inputs[1], { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: /Guardar prediccion/ }))

    await waitFor(() => {
      expect(api.submitPrediction).toHaveBeenCalledWith('tok', {
        match_id: scheduledMatch.id,
        home_score: 2,
        away_score: 1,
      })
    })
    expect(await screen.findByText('Prediccion guardada')).toBeInTheDocument()
  })

  it('updates an existing prediction and filters saved matches', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch, lockedMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 99,
        match_id: scheduledMatch.id,
        home_score: 1,
        away_score: 1,
        points: 3,
        scored_at: null,
        created_at: '',
      },
    ])

    renderPanel()

    expect(await screen.findByText('Canadá')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Guardados' }))
    expect(screen.queryByText('Estados Unidos')).toBeNull()

    const inputs = screen.getAllByRole('spinbutton')
    fireEvent.change(inputs[0], { target: { value: '3' } })
    fireEvent.click(screen.getByRole('button', { name: /Actualizar/ }))

    await waitFor(() => {
      expect(api.updatePrediction).toHaveBeenCalledWith('tok', 99, {
        home_score: 3,
        away_score: 1,
      })
    })
  })

  it('filters match predictions by selected group', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch, lockedMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()

    // Default: group A selected — Canada (group A) visible, USA (group B) not
    expect(await screen.findByText('Canadá')).toBeInTheDocument()
    expect(screen.queryByText('Estados Unidos')).toBeNull()

    // Switch to group B
    fireEvent.click(screen.getByRole('button', { name: /^B/ }))

    expect(screen.getByText('Estados Unidos')).toBeInTheDocument()
    expect(screen.queryByText('Canadá')).toBeNull()

    // Switch back to group A
    fireEvent.click(screen.getByRole('button', { name: /^A/ }))

    expect(screen.getByText('Canadá')).toBeInTheDocument()
    expect(screen.queryByText('Estados Unidos')).toBeNull()
  })

  it('shows error state when getMatches rejects', async () => {
    vi.mocked(api.getMatches).mockRejectedValueOnce(new Error('Network error'))
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()

    expect(await screen.findByText('No se pudieron cargar los partidos')).toBeInTheDocument()
  })

  it('filters past matches with the "past" filter', async () => {
    const finishedMatch = {
      ...scheduledMatch,
      id: 15,
      home_team: 'Argentina',
      away_team: 'Chile',
      status: 'finished',
      home_score: 2,
      away_score: 1,
    }
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch, finishedMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()
    await screen.findByText('Canadá') // wait for data to load in group A

    // Switch to Past filter — only finished match (Argentina) should appear
    fireEvent.click(screen.getByRole('button', { name: 'Pasados' }))
    expect(await screen.findByText('Argentina')).toBeInTheDocument()
    expect(screen.queryByText('Canadá')).toBeNull()
  })

  it('shows today-filter empty state when no matches are scheduled today', async () => {
    const farFutureMatch = {
      ...scheduledMatch,
      id: 20,
      kickoff_at: '2099-12-31T18:00:00Z',
    }
    vi.mocked(api.getMatches).mockResolvedValueOnce([farFutureMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()
    await screen.findByText('Canadá') // wait for data to load

    // Switch to by-day view
    fireEvent.click(screen.getByRole('button', { name: 'Hoy' }))

    // No matches today → noTodayMatches empty state
    expect(await screen.findByText('Sin partidos para hoy')).toBeInTheDocument()
  })

  it('shows mutation error feedback on submission failure', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])
    vi.mocked(api.submitPrediction).mockRejectedValueOnce(new Error('Server error'))

    renderPanel()

    expect(await screen.findByText('Canadá')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Guardar prediccion/ }))

    expect(await screen.findByText('No se pudo guardar la prediccion')).toBeInTheDocument()
  })

  it('shows points placeholder when finished match has no scored prediction', async () => {
    const finishedMatch = {
      ...scheduledMatch,
      id: 30,
      status: 'finished',
      home_score: 1,
      away_score: 0,
    }
    vi.mocked(api.getMatches).mockResolvedValueOnce([finishedMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([])

    renderPanel()

    // Switch to past filter to see the finished match
    fireEvent.click(screen.getByRole('button', { name: 'Pasados' }))
    await screen.findByText('Canadá')

    // No prediction → points should show "–"
    expect(screen.getByText('–')).toBeInTheDocument()
  })

  it('shows allSavedToday empty state in by-day+pending view', async () => {
    const todayKickoff = new Date(Date.now() + 3 * 60 * 60 * 1000).toISOString()
    const todayMatch = {
      ...scheduledMatch,
      id: 40,
      kickoff_at: todayKickoff,
    }
    vi.mocked(api.getMatches).mockResolvedValueOnce([todayMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 1,
        match_id: todayMatch.id,
        home_score: 2,
        away_score: 0,
        points: null,
        scored_at: null,
        created_at: '',
      },
    ])

    renderPanel()
    await screen.findByText('Canadá')

    // Switch to by-day, then pending — all today's matches have predictions
    fireEvent.click(screen.getByRole('button', { name: 'Hoy' }))
    fireEvent.click(screen.getByRole('button', { name: 'Pendientes' }))

    expect(await screen.findByText('Todas las predicciones del día guardadas')).toBeInTheDocument()
  })

  it('disables score editing for locked matches and filters pending matches', async () => {
    vi.mocked(api.getMatches).mockResolvedValueOnce([scheduledMatch, lockedMatch] as never)
    vi.mocked(api.getMyPredictions).mockResolvedValueOnce([
      {
        id: 99,
        match_id: scheduledMatch.id,
        home_score: 1,
        away_score: 1,
        points: null,
        scored_at: null,
        created_at: '',
      },
    ])

    renderPanel()

    // Default: group A; Canada has a prediction
    expect(await screen.findByText('Canadá')).toBeInTheDocument()

    // Navigate to group B where the locked USA/Guatemala match lives
    fireEvent.click(screen.getByRole('button', { name: /^B/ }))
    expect(screen.getByText('Estados Unidos')).toBeInTheDocument()

    // Pendientes filter: Canada (group A, has prediction) is not shown;
    // USA (group B, in_progress) remains visible with EN VIVO badge and locked inputs
    fireEvent.click(screen.getByRole('button', { name: 'Pendientes' }))
    expect(screen.queryByText('Canadá')).toBeNull()
    expect(screen.getByText('EN VIVO')).toBeInTheDocument()
    expect(screen.getAllByRole('spinbutton')[0]).toBeDisabled()
    expect(screen.getByRole('button', { name: /Guardar prediccion/ })).toBeDisabled()
  })
})
