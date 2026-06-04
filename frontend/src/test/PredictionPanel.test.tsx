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
  starts_at: futureKickoff,
  stadium: 'Toronto Stadium',
  phase: 'Group stage',
  group_label: 'A',
}

const lockedMatch = {
  ...scheduledMatch,
  id: 11,
  home_team: 'USA',
  away_team: 'Guatemala',
  status: 'in_progress',
  starts_at: pastKickoff,
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

    expect(await screen.findByText('Canada')).toBeInTheDocument()
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

    expect(await screen.findByText('Canada')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Guardados' }))
    expect(screen.queryByText('USA')).toBeNull()

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

    expect(await screen.findByText('USA')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Pendientes' }))
    expect(screen.queryByText('Canada')).toBeNull()
    expect(screen.getByText('Bloqueado')).toBeInTheDocument()
    expect(screen.getAllByRole('spinbutton')[0]).toBeDisabled()
    expect(screen.getByRole('button', { name: /Guardar prediccion/ })).toBeDisabled()
  })
})
