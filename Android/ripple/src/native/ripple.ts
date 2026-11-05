import { Capacitor, registerPlugin } from '@capacitor/core'
import type { PluginListenerHandle } from '@capacitor/core'
import { Haptics, ImpactStyle } from '@capacitor/haptics'

export interface RippleFile { id: string; name: string; size: number | null; mimeType: string; persisted: boolean; source: 'selected' | 'received' }
export interface BrowserRoot { id: string; name: string }
export interface BrowserEntry { id: string; name: string; kind: 'directory' | 'file'; size: number | null; mimeType: string }
export interface DirectoryListing { directoryId: string; parentId: string | null; rootId: string; entries: BrowserEntry[]; hasMore: boolean; nextOffset: number | null }
export interface RipplePeer { id: string; name: string; connected: boolean }
export interface DiscoveredDevice { id: string; name: string; host: string; port: number; version: string; paired?: boolean; peerId?: string }
export interface RippleTransfer { id: string; name: string; direction: 'send' | 'receive'; state: 'offered' | 'accepted' | 'transferring' | 'verifying' | 'completed' | 'rejected' | 'paused' | 'failed' | 'cancelled'; size: number; offset: number; peerName: string }
export interface CancelTransferResult { remoteConfirmed: boolean; message?: string }
export interface RippleSettings { haptics: boolean; appearance: 'dark' | 'system'; receiveFolder: string; autoAccept: boolean }
export interface RippleState { files: RippleFile[]; peers: RipplePeer[]; discovered: DiscoveredDevice[]; transfers: RippleTransfer[]; settings: RippleSettings; discovering: boolean; discoveryError: string | null; identityFingerprint: string | null; native: boolean }
interface NativeRipple {
  getBrowserRoots(): Promise<{ roots: BrowserRoot[] }>
  addBrowserRoot(): Promise<{ root: BrowserRoot | null }>
  listDirectory(options: { directoryId: string; offset: number }): Promise<DirectoryListing>
  selectBrowserFile(options: { fileId: string }): Promise<{ file: RippleFile }>
  exitApp(): Promise<void>
  listPeers(): Promise<{ peers: RipplePeer[] }>
  getState(): Promise<RippleState>
  pickFiles(): Promise<{ files: RippleFile[] }>
  openFile(options: { fileId: string }): Promise<void>
  shareFile(options: { fileId: string }): Promise<void>
  removeFile(options: { fileId: string }): Promise<void>
  chooseReceiveFolder(): Promise<void>
  updateSettings(options: Partial<RippleSettings>): Promise<void>
  startDiscovery(): Promise<void>
  stopDiscovery(): Promise<void>
  pairFromQr(options: { invitation: string }): Promise<void>
  connectPeer(options: { peerId: string }): Promise<void>
  forgetPeer(options: { peerId: string }): Promise<void>
  offerTransfer(options: { peerId: string; fileId: string }): Promise<void>
  acceptTransfer(options: { transferId: string }): Promise<void>
  rejectTransfer(options: { transferId: string }): Promise<void>
  cancelTransfer(options: { transferId: string }): Promise<CancelTransferResult>
  setDropHover(options: { peerId: string; active: boolean }): Promise<void>
  resumeTransfer(options: { transferId: string }): Promise<void>
  queryTransfer(options: { transferId: string }): Promise<RippleTransfer>
  submitAiRequest(options: { prompt: string; fileId?: string }): Promise<void>
  approveAiAction(options: { requestId: string; actionId: string }): Promise<void>
  cancelAiRequest(options: { requestId: string }): Promise<void>
  addListener(eventName: 'stateChanged' | 'backPressed', callback: () => void): Promise<PluginListenerHandle>
}
const plugin = registerPlugin<NativeRipple>('Ripple')
export const isNative = Capacitor.isNativePlatform()
const webSettings: RippleSettings = { haptics: true, appearance: 'dark', receiveFolder: 'Choose a folder', autoAccept: false }
function requireNative() { if (!isNative) throw new Error('Open Ripple on Android to use this feature.') }
export const ripple = {
  async getBrowserRoots() { return isNative ? (await plugin.getBrowserRoots()).roots : [] },
  async addBrowserRoot() { requireNative(); return (await plugin.addBrowserRoot()).root },
  async listDirectory(directoryId: string, offset = 0) { requireNative(); return plugin.listDirectory({ directoryId, offset }) },
  async selectBrowserFile(fileId: string) { requireNative(); return (await plugin.selectBrowserFile({ fileId })).file },
  async onBack(callback: () => void) { if (!isNative) return () => {}; const handle = await plugin.addListener('backPressed', callback); return () => { void handle.remove() } },
  async exitApp() { requireNative(); return plugin.exitApp() },
  async getState(): Promise<RippleState> { return isNative ? plugin.getState() : { files: [], peers: [], discovered: [], transfers: [], settings: webSettings, discovering: false, discoveryError: null, identityFingerprint: null, native: false } },
  async subscribe(callback: () => void) { if (!isNative) return () => {}; const handle = await plugin.addListener('stateChanged', callback); return () => { void handle.remove() } },
  async listPeers() { return isNative ? (await plugin.listPeers()).peers : [] },
  async pickFiles() { requireNative(); return (await plugin.pickFiles()).files },
  async openFile(fileId: string) { requireNative(); return plugin.openFile({ fileId }) },
  async shareFile(fileId: string) { requireNative(); return plugin.shareFile({ fileId }) },
  async removeFile(fileId: string) { requireNative(); return plugin.removeFile({ fileId }) },
  async chooseReceiveFolder() { requireNative(); return plugin.chooseReceiveFolder() },
  async updateSettings(settings: Partial<RippleSettings>) { requireNative(); return plugin.updateSettings(settings) },
  async startDiscovery() { requireNative(); return plugin.startDiscovery() },
  async stopDiscovery() { if (isNative) return plugin.stopDiscovery() },
  async pairFromQr(invitation: string) { requireNative(); return plugin.pairFromQr({ invitation }) },
  async scanQr() { requireNative(); const { CapacitorBarcodeScanner, CapacitorBarcodeScannerTypeHint, CapacitorBarcodeScannerAndroidScanningLibrary } = await import('@capacitor/barcode-scanner'); const result = await CapacitorBarcodeScanner.scanBarcode({ hint: CapacitorBarcodeScannerTypeHint.QR_CODE, scanInstructions: 'Scan the pairing invitation on your Linux PC', cancelButtonAccessibilityLabel: 'Cancel QR scan', android: { scanningLibrary: CapacitorBarcodeScannerAndroidScanningLibrary.ZXING } }); return result.ScanResult },
  async connectPeer(peerId: string) { requireNative(); return plugin.connectPeer({ peerId }) },
  async forgetPeer(peerId: string) { requireNative(); return plugin.forgetPeer({ peerId }) },
  async offerTransfer(peerId: string, fileId: string) { requireNative(); return plugin.offerTransfer({ peerId, fileId }) },
  async acceptTransfer(transferId: string) { requireNative(); return plugin.acceptTransfer({ transferId }) },
  async rejectTransfer(transferId: string) { requireNative(); return plugin.rejectTransfer({ transferId }) },
  async cancelTransfer(transferId: string) { requireNative(); return plugin.cancelTransfer({ transferId }) },
  async setDropHover(peerId: string, active: boolean) { requireNative(); return plugin.setDropHover({ peerId, active }) },
  async resumeTransfer(transferId: string) { requireNative(); return plugin.resumeTransfer({ transferId }) },
  async queryTransfer(transferId: string) { requireNative(); return plugin.queryTransfer({ transferId }) },
  async submitAiRequest(prompt: string, fileId?: string) { requireNative(); return plugin.submitAiRequest({ prompt, fileId }) },
  async approveAiAction(requestId: string, actionId: string) { requireNative(); return plugin.approveAiAction({ requestId, actionId }) },
  async cancelAiRequest(requestId: string) { requireNative(); return plugin.cancelAiRequest({ requestId }) },
  async haptic(enabled: boolean) { if (enabled && isNative) await Haptics.impact({ style: ImpactStyle.Light }) },
  async continuousHaptic(enabled: boolean) { if (enabled && isNative) await Haptics.vibrate({ duration: 90 }) },
}
