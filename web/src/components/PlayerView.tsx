import { useEffect, useRef, useState } from 'react'
import { downloadUrl, formatClock, mediaUrl, posterUrl } from '../api'
import type { Item } from '../types'
import { Download } from './icons'

interface PlayerViewProps {
  item: Item | null
}

const positionKey = (id: string) => `watch:position:${id}`

function readPosition(id: string): number {
  try {
    return parseFloat(localStorage.getItem(positionKey(id)) || '0') || 0
  } catch {
    return 0
  }
}

export function PlayerView({ item }: PlayerViewProps) {
  const video = useRef<HTMLVideoElement>(null)
  const [resumeAt, setResumeAt] = useState(0)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    setResumeAt(item ? readPosition(item.id) : 0)
    setPlaying(false)
  }, [item?.id])

  if (!item) {
    return (
      <div className="empty">
        <p className="empty-title">Nothing by that name on the shelf.</p>
        <p className="empty-hint">The file was moved or the library rescanned. Back to the library it is.</p>
      </div>
    )
  }

  const onTimeUpdate = () => {
    const el = video.current
    if (!el || el.currentTime === 0) return
    try {
      localStorage.setItem(positionKey(item.id), String(el.currentTime))
    } catch {
      /* storage is best-effort */
    }
  }

  const onLoadedMetadata = () => {
    const el = video.current
    if (!el) return
    if (resumeAt > 0 && resumeAt < el.duration - 5) {
      el.currentTime = resumeAt
    }
  }

  return (
    <div className="player">
      <div className="player-stage">
        <video
          ref={video}
          controls
          playsInline
          preload="metadata"
          poster={posterUrl(item.id)}
          onTimeUpdate={onTimeUpdate}
          onLoadedMetadata={onLoadedMetadata}
          onPlay={() => setPlaying(true)}
          onEnded={() => {
            setPlaying(false)
            try {
              localStorage.removeItem(positionKey(item.id))
            } catch {
              /* best-effort */
            }
          }}
        >
          <source src={mediaUrl(item.id)} type={item.mime} />
        </video>
      </div>

      <div className="player-meta">
        <h1 className="player-title">{item.title}</h1>
        <div className="player-facts">
          {item.year && <span>{item.year}</span>}
          {item.duration > 0 && <span>{formatClock(item.duration)}</span>}
          {item.width && item.height && (
            <span>
              {item.width}×{item.height}
            </span>
          )}
          {item.folder && <span>{item.folder}</span>}
        </div>
        <div className="player-actions">
          {!playing && resumeAt > 30 && (
            <button
              className="action primary"
              onClick={() => {
                video.current?.play()
              }}
            >
              Resume from {formatClock(resumeAt)}
            </button>
          )}
          {playing && resumeAt > 0 && resumeAt < (item.duration || 0) - 10 && (
            <span className="action-hint">resumed at {formatClock(resumeAt)}</span>
          )}
          <a className="action" href={downloadUrl(item.id)} download>
            <Download size={15} />
            Download
          </a>
        </div>
      </div>
    </div>
  )
}
