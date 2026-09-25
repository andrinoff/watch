import { useEffect, useMemo, useState } from 'react'
import { NavLink, Route, Routes, useLocation } from 'react-router-dom'
import { api, formatSize } from './api'
import type { Library } from './types'
import { LibraryView } from './components/LibraryView'
import { PlayerView } from './components/PlayerView'
import { RefreshCw, Search } from './components/icons'

export function App() {
  const [library, setLibrary] = useState<Library | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState(false)
  const location = useLocation()

  const load = () => {
    api.library().then(setLibrary).catch((e) => setError(e.message))
  }

  useEffect(load, [])

  // Keep the header calm on the player page: the video is the focus there.
  const isPlayer = location.pathname.startsWith('/watch/')

  const rescan = async () => {
    setBusy(true)
    try {
      await api.rescan()
      load()
    } finally {
      setBusy(false)
    }
  }

  const items = useMemo(() => {
    if (!library) return []
    const needle = query.trim().toLowerCase()
    if (!needle) return library.items
    return library.items.filter((item) => {
      const haystack = [item.title, item.folder, item.year && String(item.year)]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return haystack.includes(needle)
    })
  }, [library, query])

  const totalBytes = useMemo(
    () => (library ? library.items.reduce((sum, item) => sum + item.size, 0) : 0),
    [library],
  )

  const playerItem = useMemo(() => {
    if (!library || !isPlayer) return null
    const id = location.pathname.split('/').pop() ?? ''
    return library.items.find((item) => item.id === id) ?? null
  }, [library, isPlayer, location.pathname])

  return (
    <div className="app">
      <header className={`topbar ${isPlayer ? 'quiet' : ''}`}>
        <NavLink to="/" className="brand" aria-label="watch — back to the library">
          watch<span className="brand-dot" aria-hidden="true" />
        </NavLink>

        {!isPlayer && (
          <div className="topbar-right">
            <label className="search">
              <Search size={14} />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search the shelf"
                aria-label="Search the shelf"
                type="search"
              />
            </label>
            <button
              className="icon-btn"
              onClick={rescan}
              disabled={busy}
              aria-label="Rescan the movies folder"
              title="Rescan the movies folder"
            >
              <RefreshCw size={16} spin={busy} />
            </button>
            {library && (
              <span className="shelf-count">
                {library.items.length} {library.items.length === 1 ? 'video' : 'videos'}
                {' · '}
                {formatSize(totalBytes)}
              </span>
            )}
          </div>
        )}
        {isPlayer && (
          <NavLink to="/" className="back-link">
            <span aria-hidden="true">←</span> Library
          </NavLink>
        )}
      </header>

      <main className="content">
        {error && (
          <div className="banner" role="alert">
            {error}
            <button className="banner-retry" onClick={load}>
              Try again
            </button>
          </div>
        )}
        <Routes>
          <Route
            path="/"
            element={
              library === null ? (
                <div className="shelf-loading" aria-busy="true">
                  Reading the shelf…
                </div>
              ) : (
                <LibraryView
                  items={items}
                  query={query}
                  root={library.root}
                  total={library.items.length}
                />
              )
            }
          />
          <Route path="/watch/:id" element={<PlayerView item={playerItem} />} />
        </Routes>
      </main>
    </div>
  )
}
