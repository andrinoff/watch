export interface Item {
  id: string
  title: string
  relPath: string
  folder: string
  size: number
  modTime: string
  mime: string
  duration: number
  width?: number
  height?: number
  year?: number
  hasPoster: boolean
}

export interface Folder {
  id: string
  name: string
  relPath: string
  count: number
}

export interface Library {
  root: string
  items: Item[]
  folders: Folder[]
  scannedAt: string
}
