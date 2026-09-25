import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement> & { size?: number }

function base(size: number | undefined, props: IconProps): IconProps {
  const s = size ?? 20
  const { size: _size, ...rest } = props
  return {
    width: s,
    height: s,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 2,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    ...rest,
  }
}

export function Search(p: IconProps) {
  return (
    <svg {...base(p.size, p)}>
      <circle cx="11" cy="11" r="7" />
      <line x1="16.6" y1="16.6" x2="21" y2="21" />
    </svg>
  )
}

export function RefreshCw(p: IconProps & { spin?: boolean }) {
  const { spin, ...rest } = p
  return (
    <svg {...base(p.size, rest)} className={spin ? 'spin' : undefined}>
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.5 9a9 9 0 0 1 14.85-3.36L23 10" />
      <path d="M1 14l4.64 4.36A9 9 0 0 0 20.5 15" />
    </svg>
  )
}

export function Play(p: IconProps) {
  return (
    <svg {...base(p.size, p)}>
      <polygon points="7 4 19 12 7 20" fill="currentColor" stroke="none" />
    </svg>
  )
}

export function Download(p: IconProps) {
  return (
    <svg {...base(p.size, p)}>
      <path d="M12 3v12" />
      <polyline points="7 10 12 15 17 10" />
      <path d="M4 19h16" />
    </svg>
  )
}

export function Folder(p: IconProps) {
  return (
    <svg {...base(p.size, p)}>
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </svg>
  )
}
