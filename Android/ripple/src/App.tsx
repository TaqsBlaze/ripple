import { useCallback, useEffect, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { Icon } from './components/Icon'
import { FileBrowser } from './components/FileBrowser'
import { SplashScreen } from './components/SplashScreen'
import type { IconName } from './components/Icon'
import { ripple } from './native/ripple'
import type { RippleFile, RippleState, RippleTransfer } from './native/ripple'
import rippleIcon from './assets/icon.png'
import './App.css'

type Tab = 'Devices' | 'Transfers' | 'Assistant'
type Screen = Tab | 'pair' | 'files' | 'settings' | 'peers' | 'browser'
const tabs: Tab[] = ['Devices', 'Transfers', 'Assistant']
const terminalStates = new Set(['completed', 'rejected', 'failed', 'cancelled'])
function fileSize(bytes: number | null) {
  if (bytes === null) return 'Size unavailable'
  if (bytes < 1024) return `${bytes} B`
  const unit = bytes >= 1024 ** 3 ? 3 : bytes >= 1024 ** 2 ? 2 : 1
  return `${(bytes / 1024 ** unit).toLocaleString(undefined, { maximumFractionDigits: 1 })} ${['B', 'KB', 'MB', 'GB'][unit]}`
}
function Empty({ icon, title, children, compact = false }: { icon: IconName; title: string; children: React.ReactNode; compact?: boolean }) {
  return <div className={`empty-state ${compact ? 'compact' : ''}`}><span className="empty-icon"><Icon name={icon} size={28}/></span><h2>{title}</h2>{children}</div>
}
function App() {
  const [showSplash, setShowSplash] = useState(true)
  const [screen, setScreen] = useState<Screen>('Devices')
  const [tab, setTab] = useState<Tab>('Devices')
  const [state, setState] = useState<RippleState | null>(null)
  const [loadError, setLoadError] = useState('')
  const [busy, setBusy] = useState('')
  const [notice, setNotice] = useState('')
  const [history, setHistory] = useState(false)
  const [fileSource, setFileSource] = useState<'selected' | 'received'>('selected')
  const [filter, setFilter] = useState('All')
  const [search, setSearch] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [manual, setManual] = useState(false)
  const [invitation, setInvitation] = useState('')
  const [draft, setDraft] = useState('')
  const [browseForAssistant, setBrowseForAssistant] = useState(false)
  const [forgetId, setForgetId] = useState<string | null>(null)
  const primary = tabs.includes(screen as Tab)
  const selected = state?.files.find(file => file.id === selectedId)
  const connectedPeer = state?.peers.find(peer => peer.connected)
  const dropTargetPeer = connectedPeer ?? (state?.peers.length === 1 ? state.peers[0] : undefined)
  useEffect(() => {
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const timer = window.setTimeout(() => setShowSplash(false), reducedMotion ? 500 : 4000)
    return () => window.clearTimeout(timer)
  }, [])
  const refresh = useCallback(async () => { const next = await ripple.getState(); setState(next); setLoadError('') }, [])
  useEffect(() => {
    let alive = true
    let unsubscribe: (() => void) | undefined
    const update = () => { void ripple.getState().then(next => { if (alive) { setState(next); setLoadError('') } }).catch(error => { if (alive) setLoadError(error instanceof Error ? error.message : 'Unable to load your devices.') }) }
    update()
    void ripple.subscribe(update).then(stop => { if (alive) unsubscribe = stop; else stop() }).catch(error => { if (alive) setLoadError(error instanceof Error ? error.message : 'Unable to receive device updates.') })
    return () => { alive = false; unsubscribe?.() }
  }, [])
  useEffect(() => {
    if ((screen !== 'pair' && !state?.peers.length) || !state?.native) return
    let closed = false
    void ripple.startDiscovery().then(() => { if (closed) void ripple.stopDiscovery() }).catch(error => { if (!closed) setNotice(error instanceof Error ? error.message : 'Could not search the local network.') })
    return () => { closed = true; void ripple.stopDiscovery().catch(() => {}) }
  }, [screen, state?.native, state?.peers.length])
  const go = useCallback((next: Screen) => {
    if (next === screen) return
    setScreen(next)
    if (screen === 'pair') { setInvitation(''); setManual(false) }
    if (tabs.includes(next as Tab)) setTab(next as Tab)
    setNotice('')
    setForgetId(null)
    void ripple.haptic(state?.settings.haptics ?? false).catch(() => {})
  }, [screen, state?.settings.haptics])
  useEffect(() => {
    let alive = true
    let stop: (() => void) | undefined
    void ripple.onBack(() => {
      if (screen === 'browser') window.dispatchEvent(new Event('ripple:browser-back'))
      else if (!tabs.includes(screen as Tab)) go(screen === 'peers' ? 'settings' : tab)
      else if (tab !== 'Devices') go('Devices')
      else void ripple.exitApp()
    }).then(unsubscribe => { if (alive) stop = unsubscribe; else unsubscribe() }).catch(() => {})
    return () => { alive = false; stop?.() }
  }, [screen, tab, go])
  const run = async (label: string, action: () => Promise<void>) => {
    if (busy) return
    setBusy(label); setNotice('')
    try { await action(); await refresh() } catch (error) { setNotice(error instanceof Error ? error.message : 'Something went wrong. Please try again.') } finally { setBusy('') }
  }
  const chooseFiles = (forAssistant = false) => { setBrowseForAssistant(forAssistant); go('browser') }
  const showFiles = (source: 'selected' | 'received') => { setFileSource(source); setSearch(''); setFilter('All'); setSelectedId(null); go('files') }
  const setTransferTab = (next: boolean) => { setHistory(next); void ripple.haptic(state?.settings.haptics ?? false).catch(() => {}) }
  const transferKeyboard = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const next = event.key === 'End' || event.key === 'ArrowRight'
    setTransferTab(next)
    document.getElementById(next ? 'history-tab' : 'active-tab')?.focus()
  }
  const fileRow = (file: RippleFile) => <button className={`file-row ${selectedId === file.id ? 'selected' : ''}`} key={file.id} onClick={() => setSelectedId(file.id)} aria-pressed={selectedId === file.id}><Icon name={file.mimeType.startsWith('image/') ? 'image' : 'file'} size={27}/><span className="grow"><strong>{file.name}</strong><small>{fileSize(file.size)}{!file.persisted && file.source === 'selected' ? ' · Select again after restart' : ''}</small></span><Icon name={selectedId === file.id ? 'check' : 'chevron'} size={18}/></button>
  const transferCard = (item: RippleTransfer) => <article className="panel transfer-card" key={item.id}>{terminalStates.has(item.state) && item.canOpen ? <button className="row transfer-open" disabled={!!busy} aria-label={`Open ${item.name}`} onClick={() => void run('Opening file', () => ripple.openFile(item.id))}><Icon name={item.direction === 'send' ? 'upload' : 'download'} size={27}/><span className="grow"><strong>{item.name}</strong><small>{item.direction === 'send' ? 'To' : 'From'} {item.peerName} · {item.state === 'completed' ? 'Delivered' : item.state}</small></span><Icon name="open" size={18}/></button> : <div className="row"><Icon name={item.direction === 'send' ? 'upload' : 'download'} size={27}/><div className="grow"><strong>{item.name}</strong><small>{item.direction === 'send' ? 'To' : 'From'} {item.peerName} · {item.state === 'completed' ? 'Delivered' : item.state}</small></div></div>}{!terminalStates.has(item.state) && <><div className="percentage">{item.size ? Math.min(100, Math.floor(item.offset / item.size * 100)) : 0}%</div><progress value={item.offset} max={Math.max(1, item.size)} aria-label={`${item.name} transfer progress`}/><small>{fileSize(item.offset)} of {fileSize(item.size)}</small><div className="button-pair">{item.state === 'offered' && item.direction === 'receive' && <button className="button primary" disabled={!!busy} onClick={() => void run('Accepting transfer', () => ripple.acceptTransfer(item.id))}>Accept</button>}{item.state === 'paused' && <button className="button" disabled={!!busy} onClick={() => void run('Resuming transfer', () => ripple.resumeTransfer(item.id))}><Icon name="play" size={17}/>Resume</button>}<button className="button" disabled={!!busy} onClick={() => void run('Cancelling transfer', async () => { if (item.state === 'offered' && item.direction === 'receive') { await ripple.rejectTransfer(item.id); return } const result = await ripple.cancelTransfer(item.id); if (!result.remoteConfirmed) setNotice(result.message ?? 'Cancelled on this phone, but the PC could not be notified.') })}><Icon name="close" size={17}/>{item.state === 'offered' && item.direction === 'receive' ? 'Reject' : 'Cancel'}</button></div></>}</article>
  const activeTransfers = state?.transfers.filter(item => !terminalStates.has(item.state)) ?? []
  const pastTransfers = state?.transfers.filter(item => terminalStates.has(item.state)) ?? []
  const visibleFiles = state?.files.filter(file => file.source === fileSource && (filter === 'All' || (filter === 'Images' ? file.mimeType.startsWith('image/') : !file.mimeType.startsWith('image/'))) && file.name.toLowerCase().includes(search.toLowerCase())) ?? []
  const titles: Partial<Record<Screen, string>> = { pair: 'Pair a device', files: fileSource === 'received' ? 'Received files' : 'Selected files', settings: 'Settings', peers: 'Trusted devices', browser: 'File manager' }
  if (showSplash) return <SplashScreen/>
  return <div className="app-shell">{state?.desktopDropActive && <div className="rainbow-wave is-active" aria-hidden="true"><i/></div>}
    <header className="app-header">
      {screen === 'Devices' ? <><span className="wordmark"><img className="brand-icon" src={rippleIcon} alt=""/>ripple</span><button className="icon-button" aria-label="Settings" onClick={() => go('settings')}><Icon name="settings" size={24}/></button></> : <><div className="header-title">{!primary && <button className="icon-button" aria-label="Go back" onClick={() => screen === 'browser' ? window.dispatchEvent(new Event('ripple:browser-back')) : go(screen === 'peers' ? 'settings' : tab)}><Icon name="back"/></button>}<h1>{titles[screen] ?? screen}</h1></div>{screen === 'Assistant' && <button className="provider" onClick={() => go('settings')}><span className="status-dot"/>Host unavailable<Icon name="chevron" size={12}/></button>}</>}
    </header>
    {loadError && <div className="notice" role="alert"><span>{loadError}</span><button className="text-button" onClick={() => void run('Loading devices', refresh)}>Retry</button></div>}
    {!state && !loadError && <div className="loading-label" role="status"><span className="spinner"/>Loading your devices…</div>}
    {primary ? <main className="primary-viewport"><div className="primary-track" style={{ transform: `translateX(-${tabs.indexOf(tab) * 100}%)` }}>
      <section className="screen devices-screen" aria-label="Devices" aria-hidden={tab !== 'Devices'} inert={tab !== 'Devices'}>
        <h1 className="page-title">Your devices</h1>
        {state?.peers.length ? state.peers.map(peer => <section className="device-card panel" key={peer.id}><button className="device-info" onClick={() => void run('Connecting', () => ripple.connectPeer(peer.id))} disabled={!!busy}><Icon name="pc" size={40}/><span className="grow"><strong>{peer.name}</strong><span className={`connection ${peer.connected ? 'connected' : ''}`}><i/>{peer.connected ? 'Connected' : 'Offline'}</span><small>Trusted device</small></span><Icon name="chevron" size={17}/></button><div className="button-pair"><button className="button primary" disabled={!!busy} onClick={() => void chooseFiles()}><Icon name="upload" size={18}/>Send files</button><button className="button" onClick={() => void chooseFiles(true)} disabled={!!busy}><Icon name="chat" size={18}/>Ask a file</button></div></section>) : <section className="panel"><Empty icon="pc" title="Your devices, connected"><p>Pair your Linux PC to send files securely over your local network.</p><button className="button primary" onClick={() => go('pair')}><Icon name="qr" size={18}/>Pair a device</button></Empty></section>}
        <div className="section-heading"><h2>On this device</h2><button className="text-button" onClick={() => showFiles('selected')}>View files</button></div>
        <button className="file-row" disabled={!!busy} onClick={() => void chooseFiles()}><Icon name="folder" size={27}/><span className="grow"><strong>Choose files</strong><small>{state?.files.filter(file => file.source === 'selected').length || 0} selected · originals stay on your device</small></span><Icon name="chevron" size={17}/></button>
        <div className="section-heading"><h2>Recent transfers</h2><button className="text-button" onClick={() => showFiles('received')}>Received files</button></div>
        {pastTransfers.length ? pastTransfers.slice(0, 3).map(transferCard) : <Empty icon="transfer" title="Nothing transferred yet" compact><p>Your verified deliveries will appear here.</p></Empty>}
        {!!state?.peers.length && <button className="pair-link" onClick={() => go('pair')}><Icon name="qr" size={18}/>Pair another device</button>}
      </section>
      <section className="screen transfers-screen" aria-label="Transfers" aria-hidden={tab !== 'Transfers'} inert={tab !== 'Transfers'}>
        <div className="segmented" role="tablist" aria-label="Transfer view"><span className="segmented-indicator" style={{ transform: `translateX(${history ? 100 : 0}%)` }}/><button id="active-tab" role="tab" aria-selected={!history} aria-controls="active-transfers" tabIndex={history ? -1 : 0} onKeyDown={transferKeyboard} onClick={() => setTransferTab(false)}>Active</button><button id="history-tab" role="tab" aria-selected={history} aria-controls="transfer-history" tabIndex={history ? 0 : -1} onKeyDown={transferKeyboard} onClick={() => setTransferTab(true)}>History</button></div>
        <div className="transfer-viewport"><div className="transfer-track" style={{ transform: `translateX(-${history ? 100 : 0}%)` }}><div className="transfer-page" id="active-transfers" role="tabpanel" aria-labelledby="active-tab" aria-hidden={history} inert={history}>{activeTransfers.length ? activeTransfers.map(transferCard) : <Empty icon="transfer" title="No active transfers"><p>Choose a file and a paired device to start a transfer.</p><button className="button" disabled={!!busy} onClick={() => void chooseFiles()}><Icon name="upload" size={18}/>Choose files</button></Empty>}</div><div className="transfer-page" id="transfer-history" role="tabpanel" aria-labelledby="history-tab" aria-hidden={!history} inert={!history}>{pastTransfers.length ? pastTransfers.map(transferCard) : <Empty icon="check" title="Your transfer history"><p>Completed, cancelled and failed transfers will appear here.</p></Empty>}</div></div></div>
      </section>
      <section className="screen assistant-screen" aria-label="Assistant" aria-hidden={tab !== 'Assistant'} inert={tab !== 'Assistant'}>
        <Empty icon="chat" title="Ask about your files"><p>Connect your Linux PC to ask questions, find files and get answers with sources.</p><button className="button" onClick={() => go('pair')}>Connect your PC</button></Empty>
        <div className="composer-area"><button className="find-chip" onClick={() => { setDraft('Find a file on my PC'); document.getElementById('assistant-message')?.focus() }}><Icon name="search" size={16}/>Find a file on my PC</button>{selected && <div className="attachment"><Icon name="file" size={20}/><strong className="grow">{selected.name}</strong><button className="icon-button" aria-label="Remove attachment" onClick={() => setSelectedId(null)}><Icon name="close" size={17}/></button></div>}<form className="composer" onSubmit={event => { event.preventDefault(); if (draft.trim()) void run('Contacting your PC', () => ripple.submitAiRequest(draft.trim(), selected?.id)) }}><button type="button" className="icon-button" aria-label="Choose attachment" disabled={!!busy} onClick={() => void chooseFiles(true)}><Icon name="attach"/></button><input id="assistant-message" aria-label="Message Ripple" placeholder="Ask Ripple…" value={draft} onChange={event => setDraft(event.target.value)}/><button className="icon-button" aria-label="Send message" disabled={!draft.trim() || !!busy} type="submit"><Icon name="send"/></button></form></div>
      </section>
    </div></main> : <main key={screen} className={`screen secondary-screen ${screen === 'pair' ? 'pair-screen' : screen === 'browser' ? 'browser-screen' : ''}`}>
      {screen === 'browser' && <FileBrowser haptics={state?.settings.haptics ?? false} connected={!!connectedPeer} dropPeerId={!browseForAssistant ? dropTargetPeer?.id : undefined} onClose={() => go(tab)} onSelected={file => { setSelectedId(file.id); setFileSource('selected'); setSearch(''); setFilter('All'); void refresh().catch(() => {}) }} onDropSend={!browseForAssistant && dropTargetPeer ? file => { go('Transfers'); void run(`Sending ${file.name}`, () => ripple.offerTransfer(dropTargetPeer.id, file.id)) } : undefined} onDone={() => go(browseForAssistant ? 'Assistant' : 'files')}/>}
      {screen === 'pair' && <><h2>Scan the QR code on<br/>your Linux PC</h2><button className="scanner" aria-label="Scan QR code" disabled={!!busy} onClick={() => void run('Scanning QR code', async () => { const value = await ripple.scanQr(); if (value) { await ripple.pairFromQr(value); go('Devices') } })}><Icon name="qr" size={96}/><span>Tap to scan</span></button><p className="pair-help">Open Ripple on your PC and choose Pair device. Keep both devices on the same network.</p><button className="button full" aria-expanded={manual} onClick={() => setManual(!manual)}><Icon name="keyboard" size={20}/>Enter connection details</button>{manual && <form className="manual-form" onSubmit={event => { event.preventDefault(); void run('Verifying invitation', async () => { const value = invitation.trim(); setInvitation(''); await ripple.pairFromQr(value); go('Devices') }) }}><label htmlFor="invitation">Pairing invitation</label><textarea id="invitation" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={invitation} onChange={event => setInvitation(event.target.value)} required placeholder="Paste the invitation from your Linux PC"/><button className="button primary" disabled={!!busy || !invitation.trim()} type="submit">Verify invitation</button></form>}<section className="full"><div className="section-heading"><h2>Nearby devices</h2><button className="text-button" disabled={!!busy} onClick={() => void run('Searching the network', () => ripple.startDiscovery())}>Refresh</button></div>{state?.discovering && <p className="loading-label"><span className="spinner"/>Searching your network</p>}{state?.discoveryError && <p className="error-text">{state.discoveryError}</p>}{state?.discovered.map(device => <button key={device.id} className="file-row" onClick={() => { setManual(true); setNotice(device.paired ? 'This PC is already paired. Ripple is reconnecting securely.' : `Open Pair device on ${device.name} and scan its QR code to verify its identity.`) }}><Icon name="pc" size={27}/><span className="grow"><strong>{device.name}</strong><small>{device.paired ? 'Previously paired · reconnecting' : 'Nearby · pair to verify identity'}</small></span><Icon name="chevron" size={17}/></button>)}{!state?.discovered.length && <p className="pair-help">No devices found yet. You can still scan a QR code if discovery is blocked.</p>}</section></>}
      {screen === 'files' && <><div className="filters" role="group" aria-label="File location">{(['selected', 'received'] as const).map(source => <button key={source} className={fileSource === source ? 'active' : ''} aria-pressed={fileSource === source} onClick={() => { setFileSource(source); setSelectedId(null) }}>{source === 'selected' ? 'Selected' : 'Received'}</button>)}</div><label className="search-box"><Icon name="search" size={20}/><input aria-label="Search files" placeholder="Search files" value={search} onChange={event => setSearch(event.target.value)}/></label><div className="filters" role="group" aria-label="File type">{['All', 'Documents', 'Images'].map(item => <button key={item} className={filter === item ? 'active' : ''} aria-pressed={filter === item} onClick={() => setFilter(item)}>{item}</button>)}</div>{visibleFiles.map(fileRow)}{!visibleFiles.length && <Empty icon="folder" title={search || filter !== 'All' ? 'No matching files' : fileSource === 'received' ? 'No received files yet' : 'Choose your first file'}><p>{fileSource === 'received' ? 'Files saved from your paired PC will appear here.' : 'Browse your folders inside Ripple. Your originals stay where they are.'}</p>{fileSource === 'selected' && <button className="button primary" disabled={!!busy} onClick={() => void chooseFiles()}><Icon name="folder" size={18}/>Choose files</button>}</Empty>}{selected && selected.source === fileSource && <section className="file-actions panel" aria-label="Selected file actions"><div className="row"><Icon name="file" size={25}/><strong className="grow">{selected.name}</strong><button className="icon-button" aria-label="Close file actions" onClick={() => setSelectedId(null)}><Icon name="close" size={17}/></button></div><div className="action-grid"><button className="button" disabled={!!busy} onClick={() => void run('Opening file', () => ripple.openFile(selected.id))}><Icon name="open"/>Open</button><button className="button" disabled={!!busy} onClick={() => void run('Sharing file', () => ripple.shareFile(selected.id))}><Icon name="share"/>Share</button><button className="button" onClick={() => go('Assistant')}><Icon name="spark"/>Ask AI</button></div>{connectedPeer ? <button className="button primary full confirm-actions" disabled={!!busy} onClick={() => void run('Offering file', () => ripple.offerTransfer(connectedPeer.id, selected.id))}><Icon name="upload" size={18}/>Send to {connectedPeer.name}</button> : <button className="button full confirm-actions" onClick={() => go('pair')}>Pair a device to send</button>}{selected.source === 'received' ? <><button className="button full" disabled={!!busy} onClick={() => void run('Deleting received file', async () => { await ripple.removeFile(selected.id); setSelectedId(null) })}><Icon name="close"/>Delete received file</button><small className="text-center">This deletes the saved copy from the selected receive folder.</small></> : <><button className="text-button full" disabled={!!busy} onClick={() => void run('Removing selection', async () => { await ripple.removeFile(selected.id); setSelectedId(null) })}>Remove from list</button><small className="text-center">The original file will not be deleted.</small></>}</section>}{!!visibleFiles.length && fileSource === 'selected' && <button className="pair-link" disabled={!!busy} onClick={() => void chooseFiles()}><Icon name="folder" size={18}/>Choose more files</button>}</>}
      {screen === 'settings' && <><button className="settings-row" onClick={() => go('peers')}><Icon name="pc"/><span className="grow"><strong>Trusted devices</strong><small>{state?.peers.length ? `${state.peers.length} paired ${state.peers.length === 1 ? 'device' : 'devices'}` : 'No paired devices'}</small></span><Icon name="chevron" size={17}/></button><button className="settings-row" disabled={!!busy} onClick={() => void run('Choosing receive folder', () => ripple.chooseReceiveFolder())}><Icon name="folder"/><span className="grow"><strong>Receive folder</strong><small>{state?.settings.receiveFolder || 'Choose a folder'} · New files will be saved here</small></span><Icon name="chevron" size={17}/></button><div className="settings-row"><Icon name="download"/><span className="grow"><strong>Auto-accept files</strong><small>From trusted devices only · choose a receive folder first</small></span><button className={`toggle ${state?.settings.autoAccept ? 'on' : ''}`} role="switch" aria-checked={state?.settings.autoAccept ?? false} aria-label="Auto-accept files" disabled={!!busy || !state?.peers.length} onClick={() => void run('Saving receive policy', () => ripple.updateSettings({ autoAccept: !state?.settings.autoAccept }))}><span/></button></div><div className="settings-row"><Icon name="phone"/><span className="grow">Haptic feedback</span><button className={`toggle ${state?.settings.haptics ? 'on' : ''}`} role="switch" aria-checked={state?.settings.haptics ?? false} aria-label="Haptic feedback" disabled={!!busy} onClick={() => void run('Saving haptic feedback', async () => { const enabled = !state?.settings.haptics; await ripple.updateSettings({ haptics: enabled }); await ripple.haptic(enabled).catch(() => {}) })}><span/></button></div><div className="settings-row"><Icon name="chip"/><span className="grow"><strong>AI connection</strong><small>Host unavailable</small></span></div><p className="settings-note">Local and Cloud providers are managed on your Linux PC. Connect a paired PC to use the assistant.</p><div className="settings-row"><Icon name="moon"/><span><strong>Appearance</strong><small>Dark</small></span></div><p className="settings-footer">Ripple · Your files, your devices</p></>}
      {screen === 'peers' && <>{state?.peers.length ? state.peers.map(peer => <section className="device-card panel" key={peer.id}><div className="row"><Icon name="pc" size={30}/><div className="grow"><strong>{peer.name}</strong><small>{peer.connected ? 'Connected' : 'Offline'}</small></div><button className="text-button" onClick={() => setForgetId(peer.id)}>Forget</button></div>{forgetId === peer.id && <div className="help-card"><p>Forget {peer.name}? You will need to pair again before sending files.</p><div className="button-pair confirm-actions"><button className="button" onClick={() => setForgetId(null)}>Keep device</button><button className="button primary" disabled={!!busy} onClick={() => void run('Forgetting device', async () => { await ripple.forgetPeer(peer.id); setForgetId(null) })}>Forget device</button></div></div>}</section>) : <Empty icon="pc" title="No trusted devices"><p>Pair your PC to verify its identity and keep your connection private.</p><button className="button primary" onClick={() => go('pair')}><Icon name="qr" size={18}/>Pair a device</button></Empty>}{state?.identityFingerprint && <div className="help-card"><h2>This device</h2><small>Identity fingerprint</small><p className="fingerprint">{state.identityFingerprint}</p></div>}</>}
    </main>}
    {busy && <div className="loading-label" role="status"><span className="spinner"/>{busy}…</div>}
    {notice && <div className="notice" role="status"><span>{notice}</span><button className="icon-button" aria-label="Dismiss notification" onClick={() => setNotice('')}><Icon name="close" size={18}/></button></div>}
    {primary && <nav className="bottom-nav" aria-label="Main navigation"><div className="nav-track"><span className="nav-indicator" style={{ transform: `translateX(${tabs.indexOf(tab) * 100}%)` }}/>{tabs.map((item, index) => <button className={`nav-tab ${tab === item ? 'active' : ''}`} key={item} aria-current={tab === item ? 'page' : undefined} onClick={() => go(item)}><Icon name={(['pc', 'transfer', 'chat'] as const)[index]} size={22}/><span>{item}</span></button>)}</div></nav>}
  </div>
}
export default App
