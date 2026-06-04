import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { I18nProvider } from '@/lib/i18n'
import { WorldCupPoolBuilder } from '@/components/quiniela/WorldCupPoolBuilder'

function renderBuilder() {
  return render(
    <I18nProvider>
      <WorldCupPoolBuilder />
    </I18nProvider>,
  )
}

describe('WorldCupPoolBuilder', () => {
  it('renders the quiniela builder with default progress and summary', () => {
    renderBuilder()

    expect(screen.getByText('Constructor de quiniela')).toBeInTheDocument()
    expect(screen.getByText('Elige clasificados por grupo')).toBeInTheDocument()
    expect(screen.getByText('Grupo A')).toBeInTheDocument()
    expect(screen.getByText('50%')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Continuar quiniela/ })).toBeInTheDocument()
  })

  it('switches groups and completes a new group with two selections', () => {
    renderBuilder()

    fireEvent.click(screen.getByRole('button', { name: 'D' }))
    expect(screen.getByText('Los Ángeles')).toBeInTheDocument()
    expect(screen.getByText('Grupo D')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Estados Unidos/ }))
    fireEvent.click(screen.getByRole('button', { name: /Paraguay/ }))

    expect(screen.getByText('75%')).toBeInTheDocument()
    expect(screen.getAllByText('3/4')).toHaveLength(2)
    expect(screen.getAllByText('Estados Unidos')).toHaveLength(2)
    expect(screen.getAllByText('Paraguay')).toHaveLength(2)
  })

  it('keeps only two selected teams per group and allows unselecting a pick', () => {
    renderBuilder()

    fireEvent.click(screen.getByRole('button', { name: /South Africa|Sudáfrica/ }))

    const mexicoCard = screen.getByRole('button', { name: /México/ })
    const southAfricaCard = screen.getByRole('button', { name: /Sudáfrica/ })

    expect(mexicoCard).not.toHaveClass('border-gold-400/70')
    expect(southAfricaCard).toHaveClass('border-gold-400/70')

    fireEvent.click(southAfricaCard)

    expect(southAfricaCard).not.toHaveClass('border-gold-400/70')
  })
})
