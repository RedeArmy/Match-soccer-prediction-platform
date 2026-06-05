import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Providers } from '@/app/providers'
import { I18nProvider, useI18n } from '@/lib/i18n'

describe('Providers', () => {
  it('renders children inside app providers', () => {
    render(
      <Providers>
        <span>Provider child</span>
      </Providers>,
    )

    expect(screen.getByText('Provider child')).toBeInTheDocument()
  })
})

function I18nProbe() {
  const { teamName, phaseName, formatKickoff, timeZone } = useI18n()

  return (
    <div>
      <span>{teamName('Canada')}</span>
      <span>{teamName('USA')}</span>
      <span>{phaseName('group_stage')}</span>
      <span>{formatKickoff('2026-06-11T18:00:00Z')}</span>
      <span>{timeZone}</span>
    </div>
  )
}

describe('I18nProvider localisation helpers', () => {
  it('localises team names, match phases, and kickoff date-times', () => {
    render(
      <I18nProvider>
        <I18nProbe />
      </I18nProvider>,
    )

    expect(screen.getByText('Canadá')).toBeInTheDocument()
    expect(screen.getByText('Estados Unidos')).toBeInTheDocument()
    expect(screen.getByText('Fase de Grupos')).toBeInTheDocument()
    expect(screen.getByText(/2026/)).toBeInTheDocument()
  })
})
