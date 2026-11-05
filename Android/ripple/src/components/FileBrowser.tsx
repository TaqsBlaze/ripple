import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { PointerEvent as ReactPointerEvent } from 'react'
import { ripple } from '../native/ripple'
import type { BrowserEntry, BrowserRoot, DirectoryListing, RippleFile } from '../native/ripple'
import { Icon } from './Icon'
import dropEntrySound from '../../assets/sounds/universfield-swoosh.mp3'

type Drag = { entry: BrowserEntry; x: number; y: number; over: boolean }
type ReleasedWave = { id: number; y: number }
type Gesture = { entry: BrowserEntry; pointerId: number; target: HTMLButtonElement; startX: number; startY: number; lastY: number; scrolling: boolean; dragging: boolean; timer: ReturnType<typeof setTimeout> | null }
const sizeLabel = (value: number | null) => value === null ? 'File' : value < 1024 ? `${value} B` : `${(value / (value >= 1048576 ? 1048576 : 1024)).toLocaleString(undefined, { maximumFractionDigits: 1 })} ${value >= 1048576 ? 'MB' : 'KB'}`
export function FileBrowser({ onClose, onSelected, onDropSend, onDone, haptics, connected, dropPeerId }: { onClose: () => void; onSelected: (file: RippleFile) => void; onDropSend?: (file: RippleFile) => void; onDone: () => void; haptics: boolean; connected: boolean; dropPeerId?: string }) {
  const [roots, setRoots] = useState<BrowserRoot[]>([])
  const [listing, setListing] = useState<DirectoryListing | null>(null)
  const [directoryName, setDirectoryName] = useState('Your folders')
  const [selected, setSelected] = useState<BrowserEntry | null>(null)
  const [added, setAdded] = useState<RippleFile | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [drag, setDrag] = useState<Drag | null>(null)
  const [releasedWave, setReleasedWave] = useState<ReleasedWave | null>(null)
  const [search, setSearch] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)
  const dropRef = useRef<HTMLDivElement>(null)
  const gesture = useRef<Gesture | null>(null)
  const dragRef = useRef<Drag | null>(null)
  const suppressClick = useRef(false)
  const requestVersion = useRef(0)
  const alive = useRef(true)
  const vibrationTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const dropHeartbeatTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const dropAudio = useRef<HTMLAudioElement | null>(null)
  const names = useRef(new Map<string, string>())
  const clearGesture = useCallback(() => {
    const current = gesture.current
    if (dragRef.current?.over && dropPeerId) void ripple.setDropHover(dropPeerId, false).catch(() => {})
    gesture.current = null
    if (current?.timer) clearTimeout(current.timer)
    if (current?.target.hasPointerCapture(current.pointerId)) current.target.releasePointerCapture(current.pointerId)
    if (vibrationTimer.current) clearInterval(vibrationTimer.current)
    vibrationTimer.current = null
    if (dropHeartbeatTimer.current) clearInterval(dropHeartbeatTimer.current)
    dropHeartbeatTimer.current = null
    gesture.current = null; dragRef.current = null; setDrag(null)
  }, [dropPeerId])
  useEffect(() => {
    alive.current = true
    void ripple.getBrowserRoots().then(value => { if (alive.current) { setRoots(value); value.forEach(root => names.current.set(root.id, root.name)) } }).catch(reason => { if (alive.current) setError(reason instanceof Error ? reason.message : 'Could not load your folders.') }).finally(() => { if (alive.current) setLoading(false) })
    const cancel = () => clearGesture()
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') clearGesture() }
    window.addEventListener('blur', cancel); window.addEventListener('keydown', escape)
    return () => { alive.current = false; clearGesture(); window.removeEventListener('blur', cancel); window.removeEventListener('keydown', escape) }
  }, [clearGesture])
  useEffect(() => {
    if (!releasedWave) return
    const timer = setTimeout(() => setReleasedWave(null), 1400)
    return () => clearTimeout(timer)
  }, [releasedWave])
  const openDirectory = useCallback(async (id: string, name?: string, append = false, offset = 0) => {
    const version = ++requestVersion.current
    setLoading(true); setError(''); clearGesture()
    if (!append) { setSelected(null); setSearch('') }
    if (name) names.current.set(id, name)
    try {
      const result = await ripple.listDirectory(id, offset)
      if (!alive.current || version !== requestVersion.current) return
      setListing(previous => append && previous?.directoryId === id ? { ...result, entries: [...previous.entries, ...result.entries] } : result)
      setDirectoryName(names.current.get(id) || 'Folder')
      if (!append && scrollRef.current) scrollRef.current.scrollTop = 0
    } catch (reason) { if (alive.current && version === requestVersion.current) setError(reason instanceof Error ? reason.message : 'This folder is unavailable. Grant access again to continue.') } finally { if (alive.current && version === requestVersion.current) setLoading(false) }
  }, [clearGesture])
  const goBack = useCallback(() => {
    clearGesture()
    if (listing?.parentId) void openDirectory(listing.parentId)
    else if (listing) { requestVersion.current++; setLoading(false); setListing(null); setDirectoryName('Your folders'); setSelected(null); setSearch(''); setError('') }
    else onClose()
  }, [listing, clearGesture, onClose, openDirectory])
  useEffect(() => { const back = () => goBack(); window.addEventListener('ripple:browser-back', back); return () => window.removeEventListener('ripple:browser-back', back) }, [goBack])
  const addFolder = async () => {
    setLoading(true); setError('')
    try {
      const root = await ripple.addBrowserRoot()
      if (!alive.current) return
      if (root) { const next = await ripple.getBrowserRoots(); if (!alive.current) return; setRoots(next); await openDirectory(root.id, root.name) }
    } catch (reason) { if (alive.current) setError(reason instanceof Error ? reason.message : 'Folder access was not granted.') } finally { if (alive.current) setLoading(false) }
  }
  const addFile = async (entry: BrowserEntry): Promise<RippleFile | null> => {
    setLoading(true); setError('')
    try {
      const file = await ripple.selectBrowserFile(entry.id)
      if (!alive.current) return null
      setAdded(file); setSelected(null); onSelected(file)
      void ripple.haptic(haptics).catch(() => {})
      return file
    } catch (reason) { if (alive.current) setError(reason instanceof Error ? reason.message : 'Could not select this file. Try granting folder access again.') } finally { if (alive.current) setLoading(false) }
    return null
  }
  const hitDrop = (x: number, y: number) => { const rect = dropRef.current?.getBoundingClientRect(); return !!rect && x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom }
  const startGesture = (event: ReactPointerEvent<HTMLButtonElement>, entry: BrowserEntry) => {
    suppressClick.current = false
    if (entry.kind !== 'file' || loading || (event.pointerType === 'mouse' && event.button !== 0)) return
    clearGesture(); suppressClick.current = false
    const current: Gesture = { entry, pointerId: event.pointerId, target: event.currentTarget, startX: event.clientX, startY: event.clientY, lastY: event.clientY, scrolling: false, dragging: false, timer: null }
    event.currentTarget.setPointerCapture(event.pointerId)
    current.timer = setTimeout(() => {
      if (gesture.current !== current || current.scrolling) return
      current.dragging = true; current.timer = null; suppressClick.current = true
      const next = { entry, x: current.startX, y: current.startY, over: false }
      dragRef.current = next; setDrag(next); setSelected(entry)
      void ripple.haptic(haptics).catch(() => {})
    }, 360)
    gesture.current = current
  }
  const moveGesture = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const current = gesture.current
    if (!current || current.pointerId !== event.pointerId) return
    if (!current.dragging) {
      if (Math.hypot(event.clientX - current.startX, event.clientY - current.startY) > 9) {
        if (current.timer) clearTimeout(current.timer)
        current.timer = null; current.scrolling = true; suppressClick.current = true
      }
      if (current.scrolling && scrollRef.current) scrollRef.current.scrollTop += current.lastY - event.clientY
      current.lastY = event.clientY
      return
    }
    event.preventDefault()
    const over = hitDrop(event.clientX, event.clientY)
    const wasOver = dragRef.current?.over ?? false
    if (over && !wasOver && haptics) {
      void ripple.continuousHaptic(true).catch(() => {})
      vibrationTimer.current = setInterval(() => { void ripple.continuousHaptic(true).catch(() => {}) }, 520)
    } else if (!over && wasOver) {
      if (vibrationTimer.current) clearInterval(vibrationTimer.current)
      vibrationTimer.current = null
    }
    if (over && !wasOver && dropPeerId) {
      void ripple.setDropHover(dropPeerId, true).catch(() => {})
      dropHeartbeatTimer.current = setInterval(() => { void ripple.setDropHover(dropPeerId, true).catch(() => {}) }, 500)
    } else if (!over && wasOver && dropPeerId) {
      if (dropHeartbeatTimer.current) clearInterval(dropHeartbeatTimer.current)
      dropHeartbeatTimer.current = null
      void ripple.setDropHover(dropPeerId, false).catch(() => {})
    }
    if (over && !wasOver) {
      try {
        if (!dropAudio.current) { dropAudio.current = new Audio(dropEntrySound); dropAudio.current.volume = 0.55 }
        dropAudio.current.currentTime = 0
        void dropAudio.current.play().catch(() => {})
      } catch { /* Sound is optional when playback is unavailable. */ }
    }
    const next = { entry: current.entry, x: event.clientX, y: event.clientY, over }
    dragRef.current = next; setDrag(next)
    const rect = scrollRef.current?.getBoundingClientRect()
    if (!over && rect && scrollRef.current) {
      if (event.clientY < rect.top + 44) scrollRef.current.scrollTop -= 12
      else if (event.clientY > rect.bottom - 44) scrollRef.current.scrollTop += 12
    }
  }
  const endGesture = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const current = gesture.current
    if (!current || current.pointerId !== event.pointerId) return
    const shouldAdd = current.dragging && hitDrop(event.clientX, event.clientY)
    const entry = current.entry
    if (shouldAdd) {
      setReleasedWave({ id: Date.now(), y: event.clientY })
      if (haptics) void ripple.haptic(true).catch(() => {})
    }
    clearGesture()
    if (shouldAdd) void addFile(entry).then(file => { if (file && onDropSend) onDropSend(file) })
  }
  const entries = listing?.entries.filter(entry => entry.name.toLocaleLowerCase().includes(search.toLocaleLowerCase())) ?? []
  return <div className={`file-browser ${drag ? 'is-dragging' : ''}`}>
    <div className="browser-toolbar"><button className="icon-button" aria-label={listing ? 'Up one folder' : 'Back from file manager'} onClick={goBack}><Icon name="back" size={20}/></button><div className="grow"><h2>{directoryName}</h2><small>{listing ? 'Long-press to drag · tap to select' : 'Browse folders you allow Ripple to access'}</small></div><button className="icon-button" aria-label="Add folder" disabled={loading} onClick={() => void addFolder()}><Icon name="folder" size={23}/></button></div>
    {listing && <label className="search-box browser-search"><Icon name="search" size={18}/><input aria-label="Search this folder" placeholder="Search this folder" value={search} onChange={event => setSearch(event.target.value)}/></label>}
    {error && <div className="browser-error" role="alert"><p>{error}</p><button className="text-button" disabled={loading} onClick={() => listing ? void openDirectory(listing.directoryId) : void addFolder()}>{listing ? 'Try again' : 'Grant folder access'}</button></div>}
    <div className="browser-scroll" ref={scrollRef}>
      {!listing && !roots.length && !loading && <div className="empty-state"><span className="empty-icon"><Icon name="folder" size={30}/></span><h2>Your files, inside Ripple</h2><p>Allow access to a folder once, then browse its files here. Android will ask which folder you want to share.</p><button className="button primary" onClick={() => void addFolder()}>Allow a folder</button></div>}
      {!listing && !!roots.length && <div className="file-grid">{roots.map(root => <button className="file-tile folder-tile" key={root.id} disabled={loading} onClick={() => void openDirectory(root.id, root.name)}><span className="tile-icon"><Icon name="folder" size={38}/></span><strong>{root.name}</strong><small>Folder</small></button>)}</div>}
      {listing && <div className="file-grid">{entries.map(entry => <button className={`file-tile ${entry.kind === 'directory' ? 'folder-tile' : 'draggable-tile'} ${selected?.id === entry.id ? 'tile-selected' : ''} ${drag?.entry.id === entry.id ? 'tile-lifted' : ''}`} key={entry.id} disabled={loading} aria-pressed={entry.kind === 'file' ? selected?.id === entry.id : undefined} aria-label={`${entry.kind === 'directory' ? 'Open folder' : 'Select file'} ${entry.name}`} onContextMenu={event => event.preventDefault()} onPointerDown={event => startGesture(event, entry)} onPointerMove={moveGesture} onPointerUp={endGesture} onPointerCancel={clearGesture} onLostPointerCapture={() => { if (gesture.current) clearGesture() }} onClick={() => {
        if (suppressClick.current) { suppressClick.current = false; return }
        if (entry.kind === 'directory') void openDirectory(entry.id, entry.name)
        else setSelected(selected?.id === entry.id ? null : entry)
      }}><span className="tile-icon"><Icon name={entry.kind === 'directory' ? 'folder' : entry.mimeType.startsWith('image/') ? 'image' : 'file'} size={38}/></span><strong>{entry.name}</strong><small>{entry.kind === 'directory' ? 'Folder' : sizeLabel(entry.size)}</small>{selected?.id === entry.id && <span className="tile-check"><Icon name="check" size={13}/></span>}</button>)}</div>}
      {listing && !entries.length && !loading && <div className="empty-state"><Icon name="folder" size={30}/><h2>{search ? 'No matching files' : 'This folder is empty'}</h2><p>{search ? 'Try a different file name.' : 'Choose another folder to find your files.'}</p></div>}
      {loading && <p className="loading-label" role="status"><span className="spinner"/>Loading files…</p>}
      {listing?.hasMore && <button className="button full load-more" disabled={loading} onClick={() => void openDirectory(listing.directoryId, undefined, true, listing.nextOffset ?? 0)}>Load more files</button>}
    </div>
    <div ref={dropRef} className={`file-drop-edge ${drag ? 'is-visible' : ''} ${drag?.over ? 'drop-over' : ''}`} aria-label={onDropSend ? 'Drop to send to your paired PC' : 'File selection drop area'} aria-hidden={!drag}/>
    {(selected || added) && !drag && <div className="browser-actions">{selected ? <button className="button primary" disabled={loading} onClick={() => void addFile(selected)}>Add selected file</button> : added ? <><span className="grow"><strong>{added.name}</strong><small>{connected ? 'Selected · ready to send' : 'Selected · host unavailable'}</small></span><button className="button primary" onClick={onDone}>Continue</button></> : null}</div>}
    <span className="sr-only" role="status">{drag ? drag.over ? (onDropSend ? 'Over the send area. Release to send this file to your paired PC.' : 'Over the left-edge selection area. Release to select.') : `Dragging ${drag.entry.name}. Move to the left edge to ${onDropSend ? 'send it' : 'select it'}.` : added ? `${added.name} selected. Nothing has been sent.` : selected ? `${selected.name} selected. Use Add selected file or long-press to drag it to the left edge.` : ''}</span>
    {drag?.over && createPortal(<div className="rainbow-wave is-active" aria-hidden="true"><i/></div>, document.body)}
    {releasedWave && createPortal(<div key={releasedWave.id} className="rainbow-wave is-releasing" onAnimationEnd={() => setReleasedWave(null)} aria-hidden="true"><i/></div>, document.body)}
    {drag && createPortal(<div className="drag-ghost" style={{ transform: `translate3d(${drag.x - 62}px,${drag.y - 74}px,0) rotate(-5deg) scale(1.06)` }} aria-hidden="true"><Icon name={drag.entry.mimeType.startsWith('image/') ? 'image' : 'file'} size={36}/><strong>{drag.entry.name}</strong></div>, document.body)}
  </div>
}
