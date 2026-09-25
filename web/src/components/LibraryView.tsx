import { Link } from 'react-router-dom'
import { displayMeta, posterUrl } from '../api'
import type { Item } from '../types'
import { Folder } from './icons'

interface LibraryViewProps {
  items: Item[]
  query: string
  root: string
  total: number
}

export function LibraryView({ items, query, root, total }: LibraryViewProps) {
  if (total === 0) {
    return (
      <div className="empty">
        <p className="empty-title">Nothing on the shelf yet.</p>
        <p className="empty-hint">
          Drop a video into <code>{root}</code> and it shows up here within a minute —
          and on your TV at the same time.
        </p>
      </div>
    )
  }
  if (query && items.length === 0) {
    return (
      <div className="empty">
        <p className="empty-title">Nothing called “{query}”.</p>
        <p className="empty-hint">Search covers titles, folders and years.</p>
      </div>
    )
  }
  return (
    <div className="grid" role="list">
      {items.map((item) => (
        <PosterCard key={item.id} item={item} />
      ))}
    </div>
  )
}

function PosterCard({ item }: { item: Item }) {
  const meta = displayMeta(item)
  return (
    <Link
      to={`/watch/${item.id}`}
      className="card"
      aria-label={`Watch ${item.title}`}
      role="listitem"
    >
      <div className="card-art">
        <img src={posterUrl(item.id)} alt="" loading="lazy" draggable={false} />
        <div className="card-sheen" aria-hidden="true" />
      </div>
      <div className="card-info">
        <span className="card-title">{item.title}</span>
        <span className="card-meta">
          {item.folder && (
            <span className="card-folder">
              <Folder size={11} />
              {item.folder}
            </span>
          )}
          {meta && <span>{meta}</span>}
        </span>
      </div>
    </Link>
  )
}
