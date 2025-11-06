import { describe, it, expect } from 'vitest'
import { getSearchSections, filterAndSortMedia } from '../../utils/search'

describe('getSearchSections', () => {
  it('returns all items as titleMatches when search is empty', () => {
    const items = [{ title: 'A' }, { title: 'B' }]
    const res = getSearchSections(items, '')
    expect(res.titleMatches).toEqual(items)
    expect(res.overviewMatches).toEqual([])
  })

  it('splits matches between titleMatches and overviewMatches and avoids duplicates', () => {
    const items = [
      { title: 'Alpha', overview: 'first' },
      { title: 'Beta', overview: 'contains alpha inside overview' },
      { title: 'Gamma', overview: 'none' },
    ]
    const res = getSearchSections(items, 'alpha')
    expect(res.titleMatches.map((i) => i.title)).toEqual(['Alpha'])
    expect(res.overviewMatches.map((i) => i.title)).toEqual(['Beta'])
  })
})

describe('filterAndSortMedia', () => {
  it('filters out items without title and sorts by title', () => {
    const items = [
      { title: 'Charlie' },
      { title: '' },
      { title: 'Alpha' },
      {},
      { title: 'Bravo' },
    ]
    const res = filterAndSortMedia(items)
    expect(res.map((i) => i.title)).toEqual(['Alpha', 'Bravo', 'Charlie'])
  })
})
