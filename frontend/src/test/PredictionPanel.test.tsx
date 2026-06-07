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

    expect(await screen.findByText('No hay partidos programados')).toBeInTheDocument()
    expect(screen.getByText(/Cuando el calendario/)).toBeInTheDocument()
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
    // USA (group B, no prediction) remains visible and locked
    fireEvent.click(screen.getByRole('button', { name: 'Pendientes' }))
    expect(screen.queryByText('Canadá')).toBeNull()
    expect(screen.getByText('Bloqueado')).toBeInTheDocument()
    expect(screen.getAllByRole('spinbutton')[0]).toBeDisabled()
    expect(screen.getByRole('button', { name: /Guardar prediccion/ })).toBeDisabled()
  })
})
