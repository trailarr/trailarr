import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import App from '../App'
import { vi } from 'vitest'

// Mock heavy child components to keep the test focused on App wiring
vi.mock('../components/Header', () => ({ default: () => <div data-testid="header">Header</div> }))
vi.mock('../components/Sidebar', () => ({ default: () => <div data-testid="sidebar">Sidebar</div> }))
vi.mock('../components/Toast', () => ({ default: ({ message }) => <div data-testid="toast">{message}</div> }))
vi.mock('../components/HistoryPage', () => ({ default: () => <div>History</div> }))
vi.mock('../components/BlacklistPage', () => ({ default: () => <div>Blacklist</div> }))
vi.mock('../components/MediaDetails', () => ({ default: () => <div>MediaDetails</div> }))

// Mock the route component to show how many items it received
vi.mock('../MediaRouteComponent', () => ({
  default: ({ items, type }) => (
    <div data-testid="route" data-type={type}>{items ? items.length : 0}</div>
  ),
}))

// Mock API functions used by App
vi.mock('../api', () => ({
  getSeries: vi.fn().mockResolvedValue({ series: [] }),
  getMovies: vi.fn().mockResolvedValue({ movies: [{ title: 'Alpha' }] }),
  getMoviesWanted: vi.fn().mockResolvedValue({ items: [] }),
  getSeriesWanted: vi.fn().mockResolvedValue({ items: [] }),
}))

test('renders header, sidebar and loads movies via API', async () => {
  // ensure the global title setter is present so App calls it
  globalThis.setTrailarrTitle = vi.fn()

  render(
    <MemoryRouter>
      <App />
    </MemoryRouter>,
  )

  // static mocked parts
  expect(screen.getByTestId('header')).toBeInTheDocument()
  expect(screen.getByTestId('sidebar')).toBeInTheDocument()

  // wait for the mocked getMovies result to propagate into the route
  await waitFor(() => expect(screen.getByTestId('route')).toHaveTextContent('1'))

  // App should call global title setter
  expect(globalThis.setTrailarrTitle).toHaveBeenCalledWith('Movies')
})
