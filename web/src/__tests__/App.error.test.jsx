import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import App from '../App'
import { vi } from 'vitest'

// Mock heavy child components
vi.mock('../components/layout/Header', () => ({ default: () => <div data-testid="header">Header</div> }))
vi.mock('../components/layout/Sidebar', () => ({ default: () => <div data-testid="sidebar">Sidebar</div> }))
vi.mock('../components/layout/Toast', () => ({ default: ({ message }) => <div data-testid="toast">{message}</div> }))
vi.mock('../components/pages/HistoryPage', () => ({ default: () => <div>History</div> }))
vi.mock('../components/pages/BlacklistPage', () => ({ default: () => <div>Blacklist</div> }))
vi.mock('../components/media/MediaDetails', () => ({ default: () => <div>MediaDetails</div> }))

// Mock the route component to expose the `error` prop passed by App
vi.mock('../MediaRouteComponent', () => ({
  default: ({ items, error, type }) => (
    <div data-testid="route" data-type={type} data-error={error}>{items ? items.length : 0}</div>
  ),
}))

// Mock API functions used by App; make getMovies() reject
vi.mock('../api', () => ({
  getSeries: vi.fn().mockResolvedValue({ series: [] }),
  getMovies: vi.fn().mockRejectedValue(new Error('Network fail')),
  getMoviesWanted: vi.fn().mockResolvedValue({ items: [] }),
  getSeriesWanted: vi.fn().mockResolvedValue({ items: [] }),
}))

test('shows error prop on route when getMovies fails', async () => {
  globalThis.setTrailarrTitle = vi.fn()

  render(
    <MemoryRouter>
      <App />
    </MemoryRouter>,
  )

  // header/sidebar present
  expect(screen.getByTestId('header')).toBeInTheDocument()
  expect(screen.getByTestId('sidebar')).toBeInTheDocument()

  // wait for the error to be passed into the route component
  await waitFor(() => expect(screen.getByTestId('route').getAttribute('data-error')).toBe('Network fail'))
})
