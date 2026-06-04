import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Providers } from '@/app/providers'

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
