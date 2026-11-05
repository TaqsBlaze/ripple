// Exercises the shipped bundle in a DOM when a real browser is unavailable.
// This checks behavior, not rendering, animation smoothness, or native plugins.
import assert from 'node:assert/strict'
import { SourceTextModule } from 'node:vm'
import { readFile, readdir } from 'node:fs/promises'
const { JSDOM, VirtualConsole } = await import(process.env.RIPPLE_JSDOM || 'jsdom')
const errors = []
const virtualConsole = new VirtualConsole()
virtualConsole.on('jsdomError', error => errors.push(error.message))
const dom = new JSDOM('<!doctype html><div id="root"></div>', {
  url: 'https://localhost/', runScripts: 'outside-only', pretendToBeVisual: true, virtualConsole,
})
const { window } = dom
const nativeFixture = process.env.RIPPLE_NATIVE_FIXTURE === '1'
const nativeCalls = []
const listeners = new Map()
const fixtureFile = { id: 'selected-fixture', name: 'Fixture.txt', size: 32, mimeType: 'text/plain', source: 'selected', persisted: true }
const fixtureState = { files: [], peers: [], discovered: [], transfers: [], settings: { haptics: false, appearance: 'dark', receiveFolder: 'Choose a folder', autoAccept: false }, native: true, discovering: false, discoveryError: null, identityFingerprint: null }
if (nativeFixture) {
  window.androidBridge = {}
  const methods = ['getState', 'getBrowserRoots', 'listDirectory', 'selectBrowserFile', 'updateSettings', 'removeFile', 'exitApp']
  window.Capacitor = {
    PluginHeaders: [{ name: 'Ripple', methods: [...methods.map(name => ({ name, rtype: 'promise' })), { name: 'addListener', rtype: 'callback' }, { name: 'removeListener', rtype: 'promise' }] }],
    nativeCallback(_plugin, method, options, callback) { if (method === 'addListener') listeners.set(options.eventName, callback); return 'listener' },
    async nativePromise(_plugin, method, options = {}) {
      nativeCalls.push({ method, options })
      if (method === 'getState') return structuredClone(fixtureState)
      if (method === 'getBrowserRoots') return { roots: [{ id: 'root-fixture', name: 'Test folder' }] }
      if (method === 'listDirectory') {
        assert.ok(['root-fixture', 'child-fixture'].includes(options.directoryId))
        return { directoryId: options.directoryId, rootId: 'root-fixture', parentId: options.directoryId === 'root-fixture' ? null : 'root-fixture', hasMore: false, nextOffset: null,
          entries: options.directoryId === 'root-fixture' ? [{ id: 'child-fixture', name: 'Nested', kind: 'directory', size: null, mimeType: 'vnd.android.document/directory' }] : [{ id: 'file-fixture', name: fixtureFile.name, kind: 'file', size: 32, mimeType: 'text/plain' }] }
      }
      if (method === 'selectBrowserFile') { assert.equal(options.fileId, 'file-fixture'); fixtureState.files = [fixtureFile]; listeners.get('stateChanged')?.(); return { file: fixtureFile } }
      if (method === 'updateSettings') { Object.assign(fixtureState.settings, options); listeners.get('stateChanged')?.(); return }
      if (method === 'removeListener' || method === 'exitApp') return
      throw new Error(`Unexpected native call: ${method}`)
    },
  }
}
window.matchMedia = query => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {} })
window.scrollTo = () => {}
window.HTMLElement.prototype.scrollTo = () => {}
window.requestAnimationFrame = cb => window.setTimeout(() => cb(performance.now()), 0)
window.cancelAnimationFrame = id => window.clearTimeout(id)
window.ResizeObserver = class { observe() {} disconnect() {} unobserve() {} }
const bundle = (await readdir('dist/assets')).find(name => /^index-.*\.js$/.test(name))
assert.ok(bundle, 'Build the production web bundle before running this test')
const module = new SourceTextModule(await readFile(`dist/assets/${bundle}`, 'utf8'), {
  context: dom.getInternalVMContext(),
  initializeImportMeta(meta) { meta.url = `https://localhost/assets/${bundle}` },
})
await module.link(() => { throw new Error('Unexpected static module dependency') })
await module.evaluate()
const tick = () => new Promise(resolve => setTimeout(resolve, 80))
await new Promise(resolve => setTimeout(resolve, 4100))
const visible = node => !node.closest('[inert], [aria-hidden="true"], [hidden]')
const queryButton = name => [...window.document.querySelectorAll('button')].find(node => visible(node) && (node.getAttribute('aria-label') === name || node.textContent.trim() === name))
const click = async name => {
  const button = queryButton(name)
  assert.ok(button, `Missing button: ${name}`)
  button.click()
  await tick()
}
const pageText = () => [...window.document.querySelectorAll('h1,h2,h3,p')].filter(visible).map(node => node.textContent).join(' ')
assert.ok(window.document.getElementById('root').textContent.length > 0, 'Application rendered')
assert.doesNotMatch(window.document.body.textContent, /DESIGN PREVIEW|Brand-assets.zip|Proposal.pdf|Invoice.pdf|Design.png|SAMPLE RESPONSE|icon pending/)
await click('Transfers')
assert.match(pageText(), /transfer/i)
await click('History')
const history = queryButton('History')
assert.equal(history.getAttribute('aria-selected'), 'true')
await click('Active')
assert.equal(queryButton('Active').getAttribute('aria-selected'), 'true')
await click('Assistant')
assert.match(pageText(), /host|connect|pair/i)
await click('Devices')
await click('Settings')
assert.match(pageText(), /Settings/)
if (nativeFixture) {
  await click('Go back')
  const choose = [...window.document.querySelectorAll('button')].find(node => visible(node) && node.textContent.includes('Choose files'))
  assert.ok(choose); choose.click(); await tick()
  await click('Test folderFolder')
  await click('Open folder Nested')
  assert.ok(queryButton('Select file Fixture.txt'))
  const drop = window.document.querySelector('[aria-label="File selection drop area"]')
  assert.ok(drop)
  drop.getBoundingClientRect = () => ({ left: 0, right: 60, top: 0, bottom: 800, width: 60, height: 800 })
  const tile = queryButton('Select file Fixture.txt')
  let capture = false
  tile.setPointerCapture = () => { capture = true }
  tile.hasPointerCapture = () => capture
  tile.releasePointerCapture = () => { capture = false }
  const pointer = (type, x, y) => {
    const event = new window.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0 })
    Object.defineProperties(event, { pointerId: { value: 1 }, pointerType: { value: 'touch' } })
    tile.dispatchEvent(event)
  }
  // Moving before the long-press threshold must scroll, not lift the tile.
  pointer('pointerdown', 70, 150); pointer('pointermove', 70, 180)
  await new Promise(resolve => setTimeout(resolve, 400))
  assert.equal(window.document.querySelector('.drag-ghost'), null)
  pointer('pointerup', 70, 180); await tick()
  // Entering the drop area gives feedback, but cancellation must select nothing.
  pointer('pointerdown', 70, 150)
  await new Promise(resolve => setTimeout(resolve, 400))
  assert.ok(window.document.querySelector('.drag-ghost'))
  pointer('pointermove', 30, 400); await tick()
  assert.ok(window.document.querySelector('.rainbow-wave'))
  pointer('pointercancel', 30, 400); await tick()
  assert.equal(window.document.querySelector('.drag-ghost'), null)
  assert.equal(nativeCalls.filter(call => call.method === 'selectBrowserFile').length, 0)
  pointer('pointerdown', 70, 150)
  await new Promise(resolve => setTimeout(resolve, 400))
  pointer('pointermove', 30, 400); pointer('pointerup', 30, 400); tile.click(); await tick()
  assert.equal(nativeCalls.filter(call => call.method === 'selectBrowserFile').length, 1)
  assert.equal(window.document.querySelector('.drag-ghost'), null)
  assert.ok(window.document.body.textContent.includes('Selected · host unavailable'))
  assert.equal(nativeCalls.some(call => call.method === 'offerTransfer'), false)
  await click('Select file Fixture.txt')
  await click('Add selected file')
  assert.equal(nativeCalls.filter(call => call.method === 'selectBrowserFile').length, 2)
  await click('Continue')
  assert.ok(window.document.body.textContent.includes('Selected files'))
}
assert.deepEqual(errors, [])
dom.window.close()
console.log(`${nativeFixture ? 'Native bridge fixture interaction test. ' : ''}PASS: production bundle renders; no sample data; bottom navigation, transfer tabs and settings work in DOM. Native and rendered visual checks remain separate.`)
