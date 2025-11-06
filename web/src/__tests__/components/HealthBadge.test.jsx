import React from 'react'
import { render, screen } from '@testing-library/react'
import HealthBadge from '../../components/HealthBadge'

describe('HealthBadge', () => {
  it('renders null when count is zero or negative', () => {
    const { container: c1 } = render(<HealthBadge count={0} />)
    expect(c1.firstChild).toBeNull()
    const { container: c2 } = render(<HealthBadge count={-1} />)
    expect(c2.firstChild).toBeNull()
  })

  it('renders number and aria-label for positive counts', () => {
    render(<HealthBadge count={5} />)
    const el = screen.getByLabelText('5 health issues')
    expect(el).toBeInTheDocument()
    expect(el.textContent).toBe('5')
  })

  it('caps display at 9+', () => {
    render(<HealthBadge count={12} />)
    const el = screen.getByLabelText('12 health issues')
    expect(el.textContent).toBe('9+')
  })
})
