import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render as rtlRender, screen, waitFor } from '@testing-library/react'
import React from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@/lib/i18n'
import { PendingApprovalsDialog } from '@/components/groups/PendingApprovalsDialog'
import { api } from '@/lib/api'

vi.mock('@clerk/nextjs', () => ({
  useAuth: vi.fn().mockReturnValue({ getToken: vi.fn().mockResolvedValue('tok') }),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getGroupMembers:    vi.fn(),
    approveGroupMember: vi.fn(),
    rejectGroupMember:  vi.fn(),
  },
}))

function render(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return rtlRender(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>,
  )
}

const BASE = { groupId: 1, groupName: 'Mis Amigos', onClose: vi.fn() }

const pendingMember = {
  id: 10,
  user_id: 42,
  display_name: 'Juan Pérez',
  status: 'pending',
  role: 'member',
  paid: false,
  points: 0,
}

describe('PendingApprovalsDialog', () => {
  beforeEach(() => {
    vi.mocked(api.getGroupMembers).mockReset()
    vi.mocked(api.approveGroupMember).mockReset()
    vi.mocked(api.rejectGroupMember).mockReset()
    BASE.onClose = vi.fn()
  })

  it('renders nothing when open=false', () => {
    const { container } = render(<PendingApprovalsDialog {...BASE} open={false} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows group name in header when open=true', async () => {
    vi.mocked(api.getGroupMembers).mockResolvedValue([])
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    expect(screen.getByText('Mis Amigos')).toBeInTheDocument()
    expect(screen.getByText('Solicitudes pendientes')).toBeInTheDocument()
  })

  it('shows loading spinner while fetching', () => {
    vi.mocked(api.getGroupMembers).mockReturnValue(new Promise(() => {}))
    const { container } = render(<PendingApprovalsDialog {...BASE} open={true} />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows "no pending" message when list is empty', async () => {
    vi.mocked(api.getGroupMembers).mockResolvedValue([])
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    expect(await screen.findByText('No hay solicitudes pendientes.')).toBeInTheDocument()
  })

  it('filters out active members and renders only pending ones', async () => {
    const active = { ...pendingMember, id: 99, display_name: 'Carlos', status: 'active' }
    vi.mocked(api.getGroupMembers).mockResolvedValue([pendingMember, active] as never)
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    expect(await screen.findByText('Juan Pérez')).toBeInTheDocument()
    expect(screen.queryByText('Carlos')).toBeNull()
  })

  it('renders approve and reject buttons for each pending member', async () => {
    vi.mocked(api.getGroupMembers).mockResolvedValue([pendingMember] as never)
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    await screen.findByText('Juan Pérez')
    expect(screen.getByRole('button', { name: /Aceptar/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Rechazar/ })).toBeInTheDocument()
  })

  it('calls approveGroupMember with correct args on approve click', async () => {
    vi.mocked(api.getGroupMembers).mockResolvedValue([pendingMember] as never)
    vi.mocked(api.approveGroupMember).mockResolvedValue(undefined as never)
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    await screen.findByText('Juan Pérez')
    fireEvent.click(screen.getByRole('button', { name: /Aceptar/ }))
    await waitFor(() => {
      expect(api.approveGroupMember).toHaveBeenCalledWith('tok', 1, 10)
    })
  })

  it('calls rejectGroupMember with correct args on reject click', async () => {
    vi.mocked(api.getGroupMembers).mockResolvedValue([pendingMember] as never)
    vi.mocked(api.rejectGroupMember).mockResolvedValue(undefined as never)
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    await screen.findByText('Juan Pérez')
    fireEvent.click(screen.getByRole('button', { name: /Rechazar/ }))
    await waitFor(() => {
      expect(api.rejectGroupMember).toHaveBeenCalledWith('tok', 1, 10)
    })
  })

  it('calls onClose when backdrop is clicked', async () => {
    const onClose = vi.fn()
    vi.mocked(api.getGroupMembers).mockResolvedValue([])
    const { container } = render(<PendingApprovalsDialog {...BASE} open={true} onClose={onClose} />)
    await screen.findByText('No hay solicitudes pendientes.')
    fireEvent.click(container.querySelector('.absolute.inset-0')!)
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('calls onClose when X button is clicked', async () => {
    const onClose = vi.fn()
    vi.mocked(api.getGroupMembers).mockResolvedValue([])
    render(<PendingApprovalsDialog {...BASE} open={true} onClose={onClose} />)
    await screen.findByText('No hay solicitudes pendientes.')
    fireEvent.click(screen.getByRole('button', { name: 'Cerrar' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('renders multiple pending members', async () => {
    const second = { ...pendingMember, id: 11, display_name: 'Ana López' }
    vi.mocked(api.getGroupMembers).mockResolvedValue([pendingMember, second] as never)
    render(<PendingApprovalsDialog {...BASE} open={true} />)
    expect(await screen.findByText('Juan Pérez')).toBeInTheDocument()
    expect(screen.getByText('Ana López')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Aceptar/ })).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: /Rechazar/ })).toHaveLength(2)
  })

})
