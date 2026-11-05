package com.openproject.ripple.ripple

import android.app.Activity
import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper
import android.provider.DocumentsContract
import android.provider.OpenableColumns
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.activity.result.ActivityResult
import com.getcapacitor.JSArray
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.ActivityCallback
import com.getcapacitor.annotation.CapacitorPlugin
import org.json.JSONArray
import org.json.JSONObject
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Signature
import android.util.Base64
import java.security.spec.ECGenParameterSpec
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.TimeUnit

@CapacitorPlugin(name = "Ripple")
class RipplePlugin : Plugin() {
    private val worker = Executors.newSingleThreadExecutor()
    private val transferWorker = Executors.newCachedThreadPool()
    private val dropWorker = Executors.newSingleThreadExecutor()
    private val incomingMonitor = Executors.newSingleThreadScheduledExecutor()
    private data class BrowserNode(val uri: Uri, val rootId: String, val parentId: String?, val name: String, val mimeType: String, val size: Long?)
    private val browserRoots = LinkedHashMap<String, JSONObject>()
    private val browserNodes = LinkedHashMap<String, BrowserNode>()
    private val selected = ConcurrentHashMap<String, JSONObject>()
    private val discovered = ConcurrentHashMap<String, JSObject>()
    private val resolutionQueue = ConcurrentLinkedQueue<NsdServiceInfo>()
    private val seenServices = ConcurrentHashMap.newKeySet<String>()
    @Volatile private var resolving = false
    private val prefs get() = context.getSharedPreferences("ripple_local", Context.MODE_PRIVATE)
    private val trustedPeers = ConcurrentHashMap<String, JSONObject>()
    private val peerOnlineUntil = ConcurrentHashMap<String, Long>()
    private val transferRecords = ConcurrentHashMap<String, JSONObject>()
    private val cancellationRequests = ConcurrentHashMap.newKeySet<String>()
    private val activeSendTransfers = ConcurrentHashMap.newKeySet<String>()
    private val activeReceiveTransfers = ConcurrentHashMap.newKeySet<String>()
    @Volatile private var monitorEnabled = true
    @Volatile private var monitorStarted = false
    @Volatile private var desktopDropActive = false
    @Volatile private var listener: NsdManager.DiscoveryListener? = null
    @Volatile private var discoveryGeneration = 0
    @Volatile private var discovering = false
    @Volatile private var discoveryError: String? = null
    @Volatile private var pickerPending = false
    private var fingerprint: String? = null
    private val nsd get() = context.getSystemService(Context.NSD_SERVICE) as NsdManager

    override fun load() {
        runCatching { JSONArray(prefs.getString("trustedPeers", "[]")) }.getOrElse { JSONArray() }.let { peers ->
            for (i in 0 until peers.length()) peers.optJSONObject(i)?.let { peer ->
                if (peer.optString("serverId").isEmpty()) peer.put("serverId", peer.optString("fingerprint").lowercase().replace(" ", "").take(32))
                trustedPeers[peer.optString("id")] = peer
            }
        }
        runCatching { JSONArray(prefs.getString("transfers", "[]")) }.getOrElse { JSONArray() }.let { items ->
            for (i in 0 until items.length()) items.optJSONObject(i)?.let { item -> transferRecords[item.optString("id")] = item }
        }
        val roots = runCatching { JSONArray(prefs.getString("browserRoots", "[]")) }.getOrElse { JSONArray() }
        for (i in 0 until roots.length()) {
            val root = roots.optJSONObject(i) ?: continue
            val uri = runCatching { Uri.parse(root.getString("uri")) }.getOrNull() ?: continue
            if (context.contentResolver.persistedUriPermissions.any { it.uri == uri && it.isReadPermission }) {
                val id = root.optString("id")
                browserRoots[id] = root
                browserNodes[id] = BrowserNode(DocumentsContract.buildDocumentUriUsingTree(uri, DocumentsContract.getTreeDocumentId(uri)), id, null, root.optString("name"), DocumentsContract.Document.MIME_TYPE_DIR, null)
            }
        }
        val saved = runCatching { JSONArray(prefs.getString("files", "[]")) }.getOrElse { JSONArray() }
        for (i in 0 until saved.length()) {
            val file = saved.optJSONObject(i) ?: continue
            val uri = runCatching { Uri.parse(file.getString("uri")) }.getOrNull() ?: continue
            if (context.contentResolver.persistedUriPermissions.any { it.uri == uri && it.isReadPermission } || browserRoots.containsKey(file.optString("rootId"))) selected[file.getString("id")] = file
        }
        if (!monitorStarted) {
            monitorStarted = true
            incomingMonitor.scheduleWithFixedDelay({ if (monitorEnabled) pollIncomingOffers() }, 1, 1, TimeUnit.SECONDS)
        }
    }

    private fun changed() { notifyListeners("stateChanged", JSObject()) }
    private fun markPeerOnline(peerId: String) {
        val validUntil = System.currentTimeMillis() + 30_000
        peerOnlineUntil[peerId] = validUntil
        Handler(Looper.getMainLooper()).postDelayed({
            if (peerOnlineUntil.remove(peerId, validUntil)) changed()
        }, 30_000)
    }
    private fun task(call: PluginCall, block: () -> Unit) {
        worker.execute {
            try { block() }
            catch (e: IllegalArgumentException) { call.reject(e.message ?: "Invalid request.", "INVALID_REQUEST") }
            catch (_: SecurityException) { call.reject("Access was denied. Choose the file or folder again in the Android picker.", "ACCESS_DENIED") }
            catch (e: Exception) { android.util.Log.e("RipplePlugin", "Native operation failed", e); call.reject(e.message?.take(240) ?: "The operation could not be completed. Please try again.", "NATIVE_ERROR") }
        }
    }
    private fun saveFiles() {
        val files = JSONArray()
        selected.values.filter { it.optBoolean("persisted") }.forEach { files.put(it) }
        check(prefs.edit().putString("files", files.toString()).commit())
    }
    private fun publicFile(file: JSONObject) = JSObject().apply {
        put("id", file.getString("id")); put("name", file.getString("name")); put("size", file.opt("size"))
        put("mimeType", file.getString("mimeType")); put("persisted", file.optBoolean("persisted")); put("source", file.optString("source", "selected"))
    }
    private fun receivedFileFor(id: String): JSONObject? = transferRecords[id]?.takeIf {
        it.optString("direction") == "receive" && it.optString("state") == "completed" && !it.optBoolean("deleted") && it.optString("uri").isNotEmpty()
    }?.let { item -> JSONObject().apply {
        put("id", id); put("name", item.optString("name")); put("size", item.optLong("size")); put("mimeType", item.optString("mimeType", "application/octet-stream"))
        put("persisted", true); put("source", "received"); put("uri", item.getString("uri"))
    } }
    private fun settings() = JSObject().apply {
        put("haptics", prefs.getBoolean("haptics", true)); put("appearance", prefs.getString("appearance", "dark"))
        put("receiveFolder", prefs.getString("receiveFolderName", "Choose a folder")); put("autoAccept", prefs.getBoolean("autoAccept", false))
    }
    private fun ensureIdentity(): String {
        fingerprint?.let { return it }
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (!store.containsAlias("ripple.identity.v1")) {
            KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").apply {
                initialize(KeyGenParameterSpec.Builder("ripple.identity.v1", KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256).build())
            }.generateKeyPair()
        }
        return MessageDigest.getInstance("SHA-256").digest(store.getCertificate("ripple.identity.v1").publicKey.encoded)
            .joinToString("") { "%02x".format(it) }.also { fingerprint = it }
    }
    private fun savePeers() {
        val peers = JSONArray(); trustedPeers.values.forEach { peers.put(it) }
        check(prefs.edit().putString("trustedPeers", peers.toString()).commit())
    }
    private fun peerViews() = JSArray().apply { trustedPeers.values.sortedBy { it.optString("name") }.forEach { peer ->
        put(JSObject().put("id", peer.optString("id")).put("name", peer.optString("name"))
            .put("connected", (peerOnlineUntil[peer.optString("id")] ?: 0L) > System.currentTimeMillis()))
    } }
    private fun transferViews() = JSArray().apply { transferRecords.values.sortedByDescending { it.optLong("createdAt", it.optLong("updatedAt")) }.forEach { item ->
        put(JSObject().put("id",item.optString("id")).put("name",item.optString("name")).put("direction",item.optString("direction"))
            .put("state",item.optString("state")).put("size",item.optLong("size")).put("offset",item.optLong("offset")).put("peerName",item.optString("peerName"))
            .put("canOpen", (item.optString("direction") == "receive" && receivedFileFor(item.optString("id")) != null) || (item.optString("direction") == "send" && item.optString("sourceUri").isNotEmpty())))
    } }
    private fun saveTransfers() { val array=JSONArray();transferRecords.values.forEach { array.put(it) };check(prefs.edit().putString("transfers",array.toString()).commit()) }

    private fun pollIncomingOffers() {
        var dirty = false
        val deviceId = prefs.getString("deviceId", null) ?: return
        trustedPeers.values.toList().forEach { peer ->
            if (!monitorEnabled) return
            try {
                val client = PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId)
                val status = client.json("GET", "/v1/peers/status")
                val updatedName = status.optString("deviceName").trim()
                if (updatedName.isNotEmpty() && updatedName.length <= 64 && updatedName != peer.optString("name")) {
                    peer.put("name", updatedName); savePeers(); changed()
                }
                val response = client.json("GET", "/v1/transfers/inbox")
                val remoteDrop = response.optBoolean("desktopDropActive", false)
                if (desktopDropActive != remoteDrop) { desktopDropActive = remoteDrop; changed() }
                val offers = response.optJSONArray("transfers") ?: JSONArray()
                for (index in 0 until offers.length()) {
                    val remote = offers.optJSONObject(index) ?: continue
                    val id = remote.optString("transferId")
                    if (id.isBlank()) continue
                    val existing = transferRecords[id]
                    if (existing != null && existing.optString("direction") != "receive") continue
                    if (existing != null && existing.optString("state") in setOf("completed", "rejected", "cancelled", "failed", "verifying") && remote.optString("state") !in setOf("completed", "rejected", "cancelled", "failed")) continue
                    val item = existing ?: JSONObject().apply {
                        put("id", id); put("peerId", peer.optString("id")); put("peerName", peer.optString("name"))
                        put("name", remote.optString("name")); put("direction", "receive"); put("mimeType", remote.optString("mimeType", "application/octet-stream"))
                        put("size", remote.optLong("size")); put("sha256", remote.optString("sha256")); put("createdAt", remote.optLong("createdAt", System.currentTimeMillis()))
                    }
                    val state = remote.optString("state", "offered")
                    val offset = remote.optLong("offset")
                    if (item.optString("state") != state || item.optLong("offset") != offset || existing == null) {
                        item.put("state", state); item.put("offset", offset); item.put("updatedAt", System.currentTimeMillis())
                        transferRecords[id] = item; dirty = true
                    }
                    if (state == "offered" && prefs.getBoolean("autoAccept", false)) startReceive(id, null)
                }
            } catch (error: Exception) {
                if (desktopDropActive) { desktopDropActive = false; changed() }
                android.util.Log.d("RipplePlugin", "Could not refresh incoming transfers from ${peer.optString("name")}", error)
            }
        }
        if (dirty) { runCatching { saveTransfers() }.onFailure { android.util.Log.e("RipplePlugin", "Could not persist incoming transfers", it) }; changed() }
    }
    private fun publicRoot(root: JSONObject) = JSObject().apply { put("id", root.getString("id")); put("name", root.getString("name")) }
    @PluginMethod fun getBrowserRoots(call: PluginCall) = task(call) {
        val roots = JSArray()
        browserRoots.values.forEach { root ->
            val uri = Uri.parse(root.getString("uri"))
            if (context.contentResolver.persistedUriPermissions.any { it.uri == uri && it.isReadPermission }) roots.put(publicRoot(root))
        }
        call.resolve(JSObject().put("roots", roots))
    }
    @PluginMethod fun addBrowserRoot(call: PluginCall) {
        activity.runOnUiThread {
            if (pickerPending) { call.reject("A folder picker is already open.", "BUSY"); return@runOnUiThread }
            pickerPending = true
            try { startActivityForResult(call, Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).apply {
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION)
            }, "browserRootPicked") } catch (_: Exception) { pickerPending = false; call.reject("No Android folder picker is available.", "PICKER_UNAVAILABLE") }
        }
    }
    @ActivityCallback private fun browserRootPicked(call: PluginCall?, result: ActivityResult) {
        pickerPending = false
        if (call == null) return
        val uri = result.data?.data
        if (result.resultCode != Activity.RESULT_OK || uri == null) { call.resolve(JSObject().put("root", JSONObject.NULL)); return }
        task(call) {
            require(browserRoots.size < 16 || browserRoots.values.any { it.optString("uri") == uri.toString() }) { "You can add up to 16 folders." }
            require(uri.scheme == "content") { "Choose an Android document folder." }
            context.contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION)
            val document = DocumentsContract.buildDocumentUriUsingTree(uri, DocumentsContract.getTreeDocumentId(uri))
            var name = "Selected folder"
            context.contentResolver.query(document, arrayOf(DocumentsContract.Document.COLUMN_DISPLAY_NAME), null, null, null)?.use { cursor -> if (cursor.moveToFirst()) name = cursor.getString(0).take(255) }
            val id = browserRoots.entries.firstOrNull { it.value.optString("uri") == uri.toString() }?.key ?: UUID.randomUUID().toString()
            val root = JSONObject().apply { put("id", id); put("uri", uri.toString()); put("name", name) }
            browserRoots[id] = root
            browserNodes[id] = BrowserNode(document, id, null, name, DocumentsContract.Document.MIME_TYPE_DIR, null)
            val roots = JSONArray(); browserRoots.values.forEach { roots.put(it) }
            check(prefs.edit().putString("browserRoots", roots.toString()).commit())
            changed(); call.resolve(JSObject().put("root", publicRoot(root)))
        }
    }
    @PluginMethod fun listDirectory(call: PluginCall) = task(call) {
        val id = call.getString("directoryId") ?: throw IllegalArgumentException("Choose a folder first.")
        val node = browserNodes[id] ?: throw IllegalArgumentException("This folder is no longer open. Start again from your folders.")
        require(node.mimeType == DocumentsContract.Document.MIME_TYPE_DIR) { "This item is not a folder." }
        val root = browserRoots[node.rootId] ?: throw IllegalArgumentException("Folder access is no longer available.")
        val tree = Uri.parse(root.getString("uri"))
        require(context.contentResolver.persistedUriPermissions.any { it.uri == tree && it.isReadPermission }) { "Folder access was removed. Add this folder again." }
        val offset = call.getInt("offset", 0) ?: 0
        require(offset in 0..10000) { "This folder is too large. Open a smaller folder." }
        val children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, DocumentsContract.getDocumentId(node.uri))
        val entries = JSArray()
        var hasMore = false
        val projection = arrayOf(DocumentsContract.Document.COLUMN_DOCUMENT_ID, DocumentsContract.Document.COLUMN_DISPLAY_NAME, DocumentsContract.Document.COLUMN_MIME_TYPE, DocumentsContract.Document.COLUMN_SIZE)
        val cursor = context.contentResolver.query(children, projection, null, null, null) ?: throw IllegalArgumentException("This folder cannot be read. Try adding it again.")
        cursor.use {
            var skipped = 0
            while (skipped < offset && it.moveToNext()) skipped++
            while (entries.length() < 100 && it.moveToNext()) {
                val documentId = it.getString(0)
                val name = (it.getString(1) ?: "Untitled").take(255)
                val mime = it.getString(2) ?: "application/octet-stream"
                val size = if (it.isNull(3)) null else it.getLong(3).takeIf { value -> value >= 0 }
                val document = DocumentsContract.buildDocumentUriUsingTree(tree, documentId)
                var childId = browserNodes.entries.firstOrNull { entry -> entry.value.rootId == node.rootId && entry.value.uri == document && entry.value.parentId == id }?.key
                if (childId == null) {
                    require(browserNodes.size < 4096) { "Too many files are open. Restart Ripple to refresh the file browser." }
                    childId = UUID.randomUUID().toString()
                }
                browserNodes[childId] = BrowserNode(document, node.rootId, id, name, mime, size)
                entries.put(JSObject().apply {
                    put("id", childId); put("name", name); put("kind", if (mime == DocumentsContract.Document.MIME_TYPE_DIR) "directory" else "file")
                    put("size", size ?: JSONObject.NULL); put("mimeType", mime)
                })
            }
            hasMore = it.moveToNext()
        }
        call.resolve(JSObject().apply {
            put("directoryId", id); put("rootId", node.rootId); put("parentId", node.parentId ?: JSONObject.NULL)
            put("entries", entries); put("hasMore", hasMore); put("nextOffset", if (hasMore) offset + entries.length() else JSONObject.NULL)
        })
    }
    @PluginMethod fun selectBrowserFile(call: PluginCall) = task(call) {
        val id = call.getString("fileId") ?: throw IllegalArgumentException("Choose a file first.")
        val node = browserNodes[id] ?: throw IllegalArgumentException("This file is no longer open. Browse to it again.")
        require(node.mimeType != DocumentsContract.Document.MIME_TYPE_DIR) { "Choose a file, not a folder." }
        val root = browserRoots[node.rootId] ?: throw IllegalArgumentException("Folder access is no longer available.")
        val tree = Uri.parse(root.getString("uri"))
        require(context.contentResolver.persistedUriPermissions.any { it.uri == tree && it.isReadPermission }) { "Folder access was removed. Add this folder again." }
        var name = node.name
        var mime = node.mimeType
        var size: Long? = null
        val cursor = context.contentResolver.query(node.uri, arrayOf(DocumentsContract.Document.COLUMN_DISPLAY_NAME, DocumentsContract.Document.COLUMN_MIME_TYPE, DocumentsContract.Document.COLUMN_SIZE), null, null, null)
            ?: throw IllegalArgumentException("This file is unavailable. Refresh the folder.")
        cursor.use {
            require(it.moveToFirst()) { "This file is no longer available. Refresh the folder." }
            name = (it.getString(0) ?: "Untitled").take(255)
            mime = it.getString(1) ?: "application/octet-stream"
            size = if (it.isNull(2)) null else it.getLong(2).takeIf { value -> value >= 0 }
        }
        require(mime != DocumentsContract.Document.MIME_TYPE_DIR) { "This item is now a folder. Refresh the folder." }
        context.contentResolver.openFileDescriptor(node.uri, "r")?.use { } ?: throw IllegalArgumentException("This file is unavailable.")
        browserNodes[id] = node.copy(name = name, mimeType = mime, size = size)
        val existing = selected.entries.firstOrNull { it.value.optString("uri") == node.uri.toString() }?.key
        require(existing != null || selected.size < 128) { "Remove an old selection before adding more files." }
        val file = JSONObject().apply {
            put("id", existing ?: UUID.randomUUID().toString()); put("name", name); put("mimeType", mime); put("size", size ?: JSONObject.NULL)
            put("persisted", true); put("uri", node.uri.toString()); put("rootId", node.rootId)
        }
        selected[file.getString("id")] = file; saveFiles(); changed(); call.resolve(JSObject().put("file", publicFile(file)))
    }
    fun dispatchBack(): Boolean {
        if (!hasListeners("backPressed")) return false
        notifyListeners("backPressed", JSObject()); return true
    }
    @PluginMethod fun exitApp(call: PluginCall) { activity.runOnUiThread { call.resolve(); activity.finish() } }
    @PluginMethod fun listPeers(call: PluginCall) { call.resolve(JSObject().put("peers", peerViews())) }
    @PluginMethod fun getState(call: PluginCall) = task(call) {
        val files = JSArray(); selected.values.sortedBy { it.optString("name") }.forEach { files.put(publicFile(it)) }
        val devices = JSArray(); discovered.values.sortedBy { it.optString("name") }.forEach { devices.put(it) }
        call.resolve(JSObject().apply {
            receivedFiles().forEach { files.put(publicFile(it)) }
            put("files", files); put("peers", peerViews()); put("discovered", devices); put("transfers", transferViews())
            put("settings", settings()); put("discovering", discovering); put("discoveryError", discoveryError ?: JSONObject.NULL)
            put("identityFingerprint", ensureIdentity()); put("desktopDropActive", desktopDropActive); put("native", true)
        })
    }
    @PluginMethod fun pickFiles(call: PluginCall) {
        activity.runOnUiThread {
            if (pickerPending) { call.reject("A file picker is already open.", "BUSY"); return@runOnUiThread }
            pickerPending = true
            try {
                startActivityForResult(call, Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                    addCategory(Intent.CATEGORY_OPENABLE); type = "*/*"; putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION)
                }, "filesPicked")
            } catch (_: Exception) { pickerPending = false; call.reject("No Android document picker is available.", "PICKER_UNAVAILABLE") }
        }
    }
    @ActivityCallback private fun filesPicked(call: PluginCall?, result: ActivityResult) {
        pickerPending = false
        if (call == null) return
        if (result.resultCode != Activity.RESULT_OK || result.data == null) { call.resolve(JSObject().put("files", JSArray())); return }
        task(call) {
            val intent = result.data!!
            val uris = mutableListOf<Uri>()
            intent.clipData?.let { clip -> for (i in 0 until clip.itemCount) uris.add(clip.getItemAt(i).uri) } ?: intent.data?.let { uris.add(it) }
            require(uris.size <= 32 && selected.size + uris.size <= 128) { "Choose up to 32 files at a time. Remove old selections if your list is full." }
            val files = JSArray()
            uris.distinct().forEach { uri ->
                require(uri.scheme == "content") { "Choose a document from an Android file provider." }
                var name = "Untitled file"
                var size: Long? = null
                context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { cursor ->
                    if (cursor.moveToFirst()) {
                        val n = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME); if (n >= 0 && !cursor.isNull(n)) name = cursor.getString(n).take(255)
                        val s = cursor.getColumnIndex(OpenableColumns.SIZE); if (s >= 0 && !cursor.isNull(s)) size = cursor.getLong(s).takeIf { it >= 0 }
                    }
                }
                var persisted = false
                if (intent.flags and Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION != 0) {
                    try { context.contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION); persisted = true } catch (_: SecurityException) { /* Some providers only grant temporary access. */ }
                }
                val id = selected.entries.firstOrNull { it.value.optString("uri") == uri.toString() }?.key ?: UUID.randomUUID().toString()
                val file = JSONObject().apply {
                    put("id", id); put("name", name); put("size", size ?: JSONObject.NULL); put("mimeType", context.contentResolver.getType(uri) ?: "application/octet-stream")
                    put("persisted", persisted); put("uri", uri.toString())
                }
                selected[id] = file; files.put(publicFile(file))
            }
            saveFiles(); changed(); call.resolve(JSObject().put("files", files))
        }
    }
    private fun fileFor(call: PluginCall): JSONObject {
        val id = call.getString("fileId") ?: throw IllegalArgumentException("Choose a file first.")
        return selected[id] ?: receivedFileFor(id) ?: transferRecords[id]?.takeIf { it.optString("direction") == "send" && it.optString("sourceUri").isNotEmpty() }?.let { item ->
            JSONObject().put("uri", item.getString("sourceUri")).put("name", item.optString("name")).put("mimeType", item.optString("mimeType", "application/octet-stream"))
        } ?: throw IllegalArgumentException("This file is no longer available. Select it again.")
    }
    private fun receivedFiles(): List<JSONObject> = transferRecords.keys.mapNotNull(::receivedFileFor).sortedBy { it.optString("name") }
    private fun launchFile(call: PluginCall, share: Boolean) = task(call) {
        val file = fileFor(call); val uri = Uri.parse(file.getString("uri"))
        context.contentResolver.openFileDescriptor(uri, "r")?.use { } ?: throw IllegalArgumentException("This file is unavailable. Choose it again.")
        val declaredType = file.optString("mimeType", "application/octet-stream").substringBefore(';').trim().lowercase()
        val providerType = context.contentResolver.getType(uri)?.substringBefore(';')?.trim()?.lowercase()
        val extension = file.optString("name").substringAfterLast('.', "").lowercase()
        val extensionType = android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension)
        val mime = listOf(declaredType, providerType, extensionType).firstOrNull { !it.isNullOrBlank() && it != "application/octet-stream" && it != "*/*" } ?: "application/octet-stream"
        val intent = Intent(if (share) Intent.ACTION_SEND else Intent.ACTION_VIEW).apply {
            if (share) { type = mime; putExtra(Intent.EXTRA_STREAM, uri) } else setDataAndType(uri, mime)
            clipData = ClipData.newRawUri("Ripple file", uri); addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }
        activity.runOnUiThread {
            try { activity.startActivity(if (share) Intent.createChooser(intent, "Share file") else intent); call.resolve() }
            catch (_: Exception) { call.reject("No installed app can open this file type.", "NO_HANDLER") }
        }
    }
    @PluginMethod fun openFile(call: PluginCall) { launchFile(call, false) }
    @PluginMethod fun shareFile(call: PluginCall) { launchFile(call, true) }
    @PluginMethod fun removeFile(call: PluginCall) = task(call) {
        val id = call.getString("fileId") ?: throw IllegalArgumentException("Choose a file first.")
        val file = selected[id]
        if (file != null) {
            selected.remove(id); saveFiles()
            if (!file.has("rootId") && file.optBoolean("persisted") && file.getString("uri") != prefs.getString("receiveFolderUri", null)) {
                runCatching { context.contentResolver.releasePersistableUriPermission(Uri.parse(file.getString("uri")), Intent.FLAG_GRANT_READ_URI_PERMISSION) }
            }
        } else {
            val received = receivedFileFor(id) ?: throw IllegalArgumentException("This received file is no longer available.")
            val storedTree = transferRecords[id]?.optString("receiveTreeUri")?.takeIf { it.isNotEmpty() }
            val fileUri = received.getString("uri")
            val treeUri = storedTree ?: context.contentResolver.persistedUriPermissions.firstOrNull { fileUri.startsWith(it.uri.toString() + "/") && it.isWritePermission }?.uri?.toString()
                ?: prefs.getString("receiveFolderUri", null) ?: throw IllegalArgumentException("The receive folder is no longer available.")
            val tree = Uri.parse(treeUri)
            require(context.contentResolver.persistedUriPermissions.any { it.uri == tree && it.isWritePermission }) { "Ripple no longer has write access to the receive folder." }
            val uri = Uri.parse(received.getString("uri"))
            require(uri.scheme == "content" && DocumentsContract.deleteDocument(context.contentResolver, uri)) { "The Android document provider could not delete this received file." }
            transferRecords[id]?.put("deleted", true)
            saveTransfers()
        }
        changed(); call.resolve()
    }
    @PluginMethod fun chooseReceiveFolder(call: PluginCall) {
        activity.runOnUiThread {
            if (pickerPending) { call.reject("A file picker is already open.", "BUSY"); return@runOnUiThread }
            pickerPending = true
            try { startActivityForResult(call, Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).apply {
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION)
            }, "folderPicked") } catch (_: Exception) { pickerPending = false; call.reject("No Android folder picker is available.", "PICKER_UNAVAILABLE") }
        }
    }
    @ActivityCallback private fun folderPicked(call: PluginCall?, result: ActivityResult) {
        pickerPending = false
        if (call == null) return
        val uri = result.data?.data
        if (result.resultCode != Activity.RESULT_OK || uri == null) { call.resolve(); return }
        task(call) {
            val flags = result.data!!.flags and (Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
            require(flags and Intent.FLAG_GRANT_WRITE_URI_PERMISSION != 0 && flags and Intent.FLAG_GRANT_READ_URI_PERMISSION != 0) { "Choose a folder with read and write access." }
            context.contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
            var name = "Selected folder"
            val document = DocumentsContract.buildDocumentUriUsingTree(uri, DocumentsContract.getTreeDocumentId(uri))
            context.contentResolver.query(document, arrayOf(DocumentsContract.Document.COLUMN_DISPLAY_NAME), null, null, null)?.use { cursor -> if (cursor.moveToFirst()) name = cursor.getString(0).take(255) }
            check(prefs.edit().putString("receiveFolderUri", uri.toString()).putString("receiveFolderName", name).commit())
            changed(); call.resolve()
        }
    }
    @PluginMethod fun updateSettings(call: PluginCall) = task(call) {
        val autoAccept = call.getBoolean("autoAccept")
        if (autoAccept == true) {
            require(trustedPeers.isNotEmpty()) { "Pair a trusted device before enabling automatic acceptance." }
            val tree = Uri.parse(prefs.getString("receiveFolderUri", null) ?: throw IllegalArgumentException("Choose a receive folder before enabling automatic acceptance."))
            require(context.contentResolver.persistedUriPermissions.any { it.uri == tree && it.isReadPermission && it.isWritePermission }) { "Choose a receive folder with read and write access before enabling automatic acceptance." }
        }
        val edit = prefs.edit()
        autoAccept?.let { edit.putBoolean("autoAccept", it) }
        call.getBoolean("haptics")?.let { edit.putBoolean("haptics", it) }
        call.getString("appearance")?.let { require(it == "dark" || it == "system") { "Unsupported appearance." }; edit.putString("appearance", it) }
        check(edit.commit()); changed(); call.resolve()
    }
    @PluginMethod fun startDiscovery(call: PluginCall) {
        activity.runOnUiThread { startDiscoveryInternal(call) }
    }
    private fun startDiscoveryInternal(call: PluginCall?) {
            if (listener != null) { call?.resolve(); return }
            discovered.clear(); resolutionQueue.clear(); seenServices.clear(); resolving = false; discoveryError = null
            val generation = ++discoveryGeneration
            val discovery = object : NsdManager.DiscoveryListener {
                override fun onDiscoveryStarted(type: String) { if (generation != discoveryGeneration) return; discovering = true; changed() }
                override fun onServiceFound(service: NsdServiceInfo) {
                    if (generation != discoveryGeneration) return
                    if (!service.serviceType.startsWith("_ripple._tcp")) return
                    if (seenServices.size < 64 && seenServices.add(service.serviceName)) {
                        resolutionQueue.add(service)
                        resolveNext(generation)
                    }
                }
                override fun onServiceLost(info: NsdServiceInfo) {
                    if (generation != discoveryGeneration) return
                    seenServices.remove(info.serviceName)
                    discovered.remove(info.serviceName)?.optString("peerId")?.takeIf { it.isNotEmpty() }?.let { peerOnlineUntil.remove(it) }
                    changed()
                }
                override fun onDiscoveryStopped(type: String) { if (generation != discoveryGeneration) return; listener = null; discovering = false; changed() }
                override fun onStartDiscoveryFailed(type: String, error: Int) { if (generation != discoveryGeneration) return; listener = null; discovering = false; discoveryError = "Local discovery is unavailable ($error). Check your network or use a QR invitation."; changed() }
                override fun onStopDiscoveryFailed(type: String, error: Int) { if (generation != discoveryGeneration) return; discovering = false; discoveryError = "Discovery could not stop ($error)."; changed() }
            }
            listener = discovery
            try { nsd.discoverServices("_ripple._tcp.", NsdManager.PROTOCOL_DNS_SD, discovery); call?.resolve() }
            catch (_: SecurityException) { listener = null; val message = "Local network access was denied. Check Android permissions and try again."; discoveryError = message; call?.reject(message, "ACCESS_DENIED"); changed() }
            catch (_: Exception) { listener = null; val message = "Discovery is unavailable. Connect both devices to the same network."; discoveryError = message; call?.reject(message, "DISCOVERY_UNAVAILABLE"); changed() }
    }
    @Synchronized
    @Suppress("DEPRECATION")
    private fun resolveNext(generation: Int) {
        if (generation != discoveryGeneration || listener == null || resolving) return
        var service = resolutionQueue.poll() ?: return
        while (!seenServices.contains(service.serviceName)) service = resolutionQueue.poll() ?: return
        val requested = service
        resolving = true
        fun finished() {
            if (generation != discoveryGeneration) return
            resolving = false
            resolveNext(generation)
        }
        try {
            nsd.resolveService(requested, object : NsdManager.ResolveListener {
                override fun onResolveFailed(info: NsdServiceInfo, error: Int) {
                    if (generation != discoveryGeneration) return
                    discoveryError = "A nearby device could not be resolved ($error). Retry discovery or use its QR invitation."
                    changed(); finished()
                }
                override fun onServiceResolved(info: NsdServiceInfo) {
                    if (generation != discoveryGeneration) return
                    val host = info.host?.hostAddress
                    if (listener != null && seenServices.contains(requested.serviceName) && host != null && info.port in 1..65535) {
                        val version = info.attributes["version"]?.toString(Charsets.UTF_8)?.take(16) ?: "unknown"
                        val serverId = info.attributes["deviceId"]?.toString(Charsets.UTF_8)?.take(32) ?: ""
                        val pairedPeer = trustedPeers.values.firstOrNull { it.optString("serverId") == serverId }
                        val endpointHost = if (host.contains(':')) "[${host.replace("%", "%25")}]" else host
                        val endpoint = "https://$endpointHost:${info.port}"
                        discovered[requested.serviceName] = JSObject().apply {
                            put("id", requested.serviceName.take(255)); put("name", pairedPeer?.optString("name") ?: "Ripple device"); put("host", host); put("port", info.port); put("version", version)
                            put("paired", pairedPeer != null); if (pairedPeer != null) put("peerId", pairedPeer.optString("id"))
                        }
                        changed()
                        if (pairedPeer != null) transferWorker.execute {
                            runCatching {
                                val deviceId = prefs.getString("deviceId", null) ?: throw IllegalStateException("Android identity is unavailable.")
                                PeerTransferClient(endpoint, pairedPeer.getString("fingerprint"), deviceId).checkOnline()
                                pairedPeer.put("endpoint", endpoint)
                                peerOnlineUntil[pairedPeer.optString("id")] = System.currentTimeMillis() + 30_000
                                savePeers()
                            }.onSuccess { markPeerOnline(pairedPeer.optString("id")); changed() }
                                .onFailure { peerOnlineUntil.remove(pairedPeer.optString("id")); changed(); android.util.Log.w("RipplePlugin", "Could not reconnect to a discovered trusted PC", it) }
                        }
                    }
                    finished()
                }
            })
        } catch (_: Exception) {
            discoveryError = "A nearby device could not be resolved. Retry discovery or use its QR invitation."
            changed(); finished()
        }
    }
    private fun stop() {
        discoveryGeneration++
        resolutionQueue.clear(); seenServices.clear(); resolving = false
        listener?.let { runCatching { nsd.stopServiceDiscovery(it) } }; listener = null; discovering = false; discovered.clear(); changed()
    }
    @PluginMethod fun stopDiscovery(call: PluginCall) { activity.runOnUiThread { stop(); call.resolve() } }
    @PluginMethod fun pairFromQr(call: PluginCall) = task(call) {
        val invitation = InvitationValidator.parse(call.getString("invitation") ?: "")
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        ensureIdentity()
        val certificate = keyStore.getCertificate("ripple.identity.v1")
        val deviceId = prefs.getString("deviceId", null) ?: UUID.randomUUID().toString().also { check(prefs.edit().putString("deviceId", it).commit()) }
        val client = PinnedEnrollmentClient(invitation)
        val start = JSONObject().apply {
            put("version", 1); put("sessionId", invitation.sessionId)
            put("secret", Base64.encodeToString(invitation.secret, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING))
            put("deviceId", deviceId); put("deviceName", android.os.Build.MODEL.take(64))
            put("publicKey", Base64.encodeToString(certificate.publicKey.encoded, Base64.NO_WRAP))
        }
        val response = client.post("/v1/enrollment/start", start)
        val enrollmentId = response.getString("enrollmentId")
        var approved: JSONObject? = null
        val deadline = System.currentTimeMillis() + 120_000
        while (System.currentTimeMillis() < deadline) {
            val status = client.post("/v1/enrollment/status", JSONObject().put("sessionId", invitation.sessionId).put("enrollmentId", enrollmentId))
            when (status.optString("state")) {
                "rejected", "expired" -> throw IllegalArgumentException("Pairing was ${status.getString("state")} on the PC. Generate a new invitation to try again.")
                "approved" -> { approved = status; break }
                "pending" -> Thread.sleep(1000)
                else -> throw IllegalArgumentException("The PC returned an unknown pairing state.")
            }
        }
        val approval = approved ?: throw IllegalArgumentException("Pairing approval timed out. Approve the request on the PC and scan a fresh invitation if needed.")
        val challenge = Base64.decode(approval.getString("challenge"), Base64.DEFAULT)
        require(challenge.size in 32..256) { "The PC returned an invalid identity challenge." }
        val signer = Signature.getInstance("SHA256withECDSA")
        signer.initSign(keyStore.getKey("ripple.identity.v1", null) as java.security.PrivateKey)
        signer.update(challenge)
        val proof = JSONObject().apply {
            put("sessionId", invitation.sessionId); put("enrollmentId", enrollmentId)
            put("deviceId", deviceId); put("signature", Base64.encodeToString(signer.sign(), Base64.NO_WRAP))
        }
        val enrolled = client.post("/v1/enrollment/proof", proof)
        require(enrolled.optString("state") == "paired") { "The PC did not confirm device enrollment." }
        val peerId = enrolled.getString("peerId")
        trustedPeers[peerId] = JSONObject().put("id", peerId).put("name", enrolled.optString("deviceName", "Ripple PC"))
            .put("serverId", invitation.fingerprint.take(32))
            .put("endpoint", invitation.endpoint.toString()).put("fingerprint", invitation.fingerprint)
        savePeers()
        runCatching { PeerTransferClient(invitation.endpoint.toString(), invitation.fingerprint, deviceId).checkOnline() }
            .onSuccess { markPeerOnline(peerId) }
            .onFailure { android.util.Log.w("RipplePlugin", "Pairing completed but PC status check failed", it) }
        changed(); call.resolve()
    }
    @PluginMethod fun connectPeer(call: PluginCall) = task(call) {
        val id = call.getString("peerId") ?: throw IllegalArgumentException("Choose a trusted PC first.")
        val peer = trustedPeers[id] ?: throw IllegalArgumentException("This PC is no longer trusted.")
        peerOnlineUntil.remove(id)
        changed()
        val deviceId = prefs.getString("deviceId", null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
        PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId).checkOnline()
        markPeerOnline(id)
        changed(); call.resolve()
    }
    @PluginMethod fun setDropHover(call: PluginCall) {
        val peerId = call.getString("peerId")
        val active = call.getBoolean("active", false) ?: false
        dropWorker.execute {
            try {
                val id = peerId ?: throw IllegalArgumentException("Choose a paired PC first.")
                val peer = trustedPeers[id] ?: throw IllegalArgumentException("This PC is no longer trusted.")
                val deviceId = prefs.getString("deviceId", null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
                PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId)
                    .json("POST", "/v1/drop-state", JSONObject().put("active", active))
                call.resolve()
            } catch (error: Exception) {
                android.util.Log.w("RipplePlugin", "Could not sync Android drop feedback to the PC", error)
                call.reject(error.message ?: "Could not sync drop feedback to the PC.", "DROP_FEEDBACK_FAILED")
            }
        }
    }
    private fun hostUnavailable(call: PluginCall) { call.reject("Host unavailable. Pair a Linux PC with a compatible Ripple service first.", "HOST_UNAVAILABLE") }
    @PluginMethod fun forgetPeer(call: PluginCall) = task(call) {
        val id = call.getString("peerId") ?: throw IllegalArgumentException("Choose a trusted PC first.")
        require(trustedPeers.remove(id) != null) { "This device is not paired." }
        savePeers(); changed(); call.resolve()
    }
    @PluginMethod fun offerTransfer(call: PluginCall) {
        transferWorker.execute {
            try { sendFile(call) }
            catch (e: IllegalArgumentException) { call.reject(e.message ?: "Invalid transfer request.", "INVALID_REQUEST") }
            catch (e: SecurityException) { call.reject("Android no longer grants access to this file. Select it again.", "ACCESS_DENIED") }
            catch (e: Exception) { call.reject(e.message ?: "The transfer failed.", "TRANSFER_FAILED") }
        }
    }
    private fun sendFile(call: PluginCall) {
        val peerId=call.getString("peerId") ?: throw IllegalArgumentException("Choose a paired PC.")
        val file=fileFor(call);val peer=trustedPeers[peerId] ?: throw IllegalArgumentException("This PC is no longer trusted.")
        val deviceId=prefs.getString("deviceId",null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
        val client=PeerTransferClient(peer.getString("endpoint"),peer.getString("fingerprint"),deviceId)
        val uri=Uri.parse(file.getString("uri"));val digest=MessageDigest.getInstance("SHA-256");var size=0L
        context.contentResolver.openInputStream(uri)?.use { input -> val buffer=ByteArray(1024*1024);while(true){val count=input.read(buffer);if(count<0)break;digest.update(buffer,0,count);size+=count} } ?: throw IllegalArgumentException("This file is unavailable. Select it again.")
        val sha256=digest.digest().joinToString("") { "%02x".format(it) };val id=UUID.randomUUID().toString()
        val record=JSONObject().apply { put("id",id);put("peerId",peerId);put("name",file.getString("name"));put("direction","send");put("state","offered");put("size",size);put("offset",0);put("peerName",peer.optString("name"));put("createdAt",System.currentTimeMillis());put("updatedAt",System.currentTimeMillis());put("sha256",sha256);put("sourceUri",uri.toString());put("mimeType",file.optString("mimeType","application/octet-stream")) }
        transferRecords[id]=record;activeSendTransfers.add(id);saveTransfers();changed()
        try {
        client.json("POST","/v1/transfers",JSONObject().apply { put("version",1);put("transferId",id);put("name",file.getString("name"));put("size",size);put("mimeType",file.optString("mimeType","application/octet-stream"));put("sha256",sha256) })
        val until=System.currentTimeMillis()+5*60*1000
        var state="offered"
        while(System.currentTimeMillis()<until) {
            if (cancellationRequests.contains(id)) throw IllegalStateException("Transfer cancelled by user.")
            val status=client.json("GET","/v1/transfers/$id")
            state=status.optString("state","offered")
            if(state=="rejected") { record.put("state","rejected");break }
            if(state=="cancelled"||state=="failed") { record.put("state",state);break }
            if(state=="accepted"||state=="transferring") break
            Thread.sleep(1000)
        }
        if(state!="accepted"&&state!="transferring") { if(state=="offered") record.put("state","failed");record.put("updatedAt",System.currentTimeMillis());saveTransfers();changed();call.resolve();return }
        record.put("state","transferring");saveTransfers();changed()
        val sourceDigest=MessageDigest.getInstance("SHA-256");var offset=0L
        context.contentResolver.openInputStream(uri)?.use { input ->
            val buffer=ByteArray(1024*1024)
            while(true){if (cancellationRequests.contains(id)) throw IllegalStateException("Transfer cancelled by user.");val count=input.read(buffer);if(count<0)break;sourceDigest.update(buffer,0,count);val chunk=if(count==buffer.size)buffer.copyOf() else buffer.copyOf(count)
                client.putChunk("/v1/transfers/$id/chunks/$offset",chunk);offset+=count;record.put("offset",offset);record.put("updatedAt",System.currentTimeMillis());saveTransfers();changed() }
        } ?: throw IllegalArgumentException("This file became unavailable during transfer.")
        if (cancellationRequests.contains(id)) throw IllegalStateException("Transfer cancelled by user.")
        val finalHash=sourceDigest.digest().joinToString("") { "%02x".format(it) }
        require(offset==size&&finalHash==sha256) { "The source file changed while it was being sent. The PC has not confirmed delivery." }
        record.put("state","verifying");saveTransfers();changed()
        val receipt=client.json("POST","/v1/transfers/$id/complete",JSONObject().put("size",size).put("sha256",sha256))
        require(receipt.optString("state")=="completed"&&receipt.optString("sha256")==sha256) { "The PC did not return a verified delivery receipt." }
        record.put("state","completed");record.put("receiptId",receipt.optString("receiptId"));record.put("updatedAt",System.currentTimeMillis());saveTransfers();changed();call.resolve()
        } catch (error: Exception) {
            if (!cancellationRequests.contains(id)) throw error
            record.put("state", "cancelled"); record.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed(); call.resolve()
        } finally {
            cancellationRequests.remove(id)
            activeSendTransfers.remove(id)
        }
    }
    private fun receiveFile(id: String, call: PluginCall?) {
        val item = transferRecords[id] ?: throw IllegalArgumentException("Transfer not found.")
        require(item.optString("direction") == "receive" && item.optString("state") == "offered") { "This file is no longer waiting for approval." }
        val peer = trustedPeers[item.optString("peerId")] ?: throw IllegalArgumentException("This PC is no longer trusted. Pair it again before receiving files.")
        val deviceId = prefs.getString("deviceId", null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
        val tree = Uri.parse(prefs.getString("receiveFolderUri", null) ?: throw IllegalArgumentException("Choose a receive folder in Settings before accepting files."))
        require(context.contentResolver.persistedUriPermissions.any { it.uri == tree && it.isReadPermission && it.isWritePermission }) { "Ripple no longer has access to the receive folder. Choose it again in Settings." }
        val client = PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId)
        val mime = item.optString("mimeType", "application/octet-stream").ifBlank { "application/octet-stream" }
        val size = item.optLong("size", -1L)
        val expectedHash = item.optString("sha256")
        require(size in 0..(10L shl 30) && expectedHash.matches(Regex("[0-9a-f]{64}"))) { "The PC sent invalid file details." }
        item.put("receiveTreeUri", tree.toString())
        val rootDocument = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
        val temporaryName = ".ripple-$id.part"
        var temporaryUri: Uri? = null
        var finalizedUri: Uri? = null
        try {
            val decision = client.json("POST", "/v1/transfers/$id/decision", JSONObject().put("approve", true))
            require(decision.optString("state") == "accepted") { "The PC did not accept this transfer." }
            item.put("state", "accepted"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
            val tempUri = DocumentsContract.createDocument(context.contentResolver, rootDocument, "application/octet-stream", temporaryName)
                ?: throw IllegalStateException("The receive folder could not create a temporary file.")
            temporaryUri = tempUri
            item.put("uri", tempUri.toString()); item.put("state", "transferring"); item.put("offset", 0); saveTransfers(); changed()
            val digest = MessageDigest.getInstance("SHA-256")
            context.contentResolver.openOutputStream(tempUri, "w")?.use { output ->
                var offset = 0L
                while (offset < size) {
                    if (cancellationRequests.contains(id)) throw IllegalStateException("Transfer cancelled by user.")
                    val path = "/v1/transfers/$id/content?offset=$offset"
                    val chunk = client.getChunk(path)
                    val expectedCount = minOf(1024L * 1024L, size - offset).toInt()
                    require(chunk.size == expectedCount) { "The PC returned an incomplete file chunk." }
                    digest.update(chunk); output.write(chunk); output.flush()
                    offset += chunk.size
                    item.put("offset", offset); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
                }
            } ?: throw IllegalStateException("The receive folder could not open the temporary file.")
            val actualHash = digest.digest().joinToString("") { "%02x".format(it) }
            require(actualHash == expectedHash) { "The file failed SHA-256 verification and was not saved as complete." }
            if (cancellationRequests.contains(id)) throw IllegalStateException("Transfer cancelled by user.")
            val safeName = safeReceiveName(tree, item.optString("name"))
            finalizedUri = DocumentsContract.renameDocument(context.contentResolver, tempUri, safeName)
                ?: throw IllegalStateException("This receive folder cannot safely finalize downloaded files. Choose a folder that supports renaming.")
            item.put("uri", finalizedUri.toString()); item.put("state", "verifying"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
            val receipt = client.json("POST", "/v1/transfers/$id/receipt", JSONObject().put("size", size).put("sha256", actualHash))
            require(receipt.optString("state") == "completed" && receipt.optString("sha256") == expectedHash) { "The PC did not confirm the verified receipt." }
            item.put("state", "completed"); item.put("receiptId", receipt.optString("receiptId")); item.put("updatedAt", System.currentTimeMillis())
            saveTransfers(); changed(); call?.resolve()
        } catch (error: Exception) {
            if (cancellationRequests.contains(id)) {
                runCatching { client.json("POST", "/v1/transfers/$id/cancel", JSONObject()) }
                temporaryUri?.let { runCatching { DocumentsContract.deleteDocument(context.contentResolver, it) } }
                item.put("state", "cancelled"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed(); call?.resolve()
            } else {
                if (finalizedUri == null) {
                    runCatching { client.json("POST", "/v1/transfers/$id/cancel", JSONObject()) }
                    temporaryUri?.let { runCatching { DocumentsContract.deleteDocument(context.contentResolver, it) } }
                }
                if (finalizedUri == null) item.remove("uri")
                item.put("state", if (finalizedUri == null) "failed" else "verifying")
                item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
                call?.reject(error.message ?: "The file could not be received.", "TRANSFER_FAILED")
            }
        } finally {
            cancellationRequests.remove(id)
        }
    }

    private fun safeReceiveName(tree: Uri, offeredName: String): String {
        val base = offeredName.replace('\\', '/').substringAfterLast('/').filter { it >= ' ' && it != '\u007f' }.take(240).ifBlank { "received-file" }
        val children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
        val existing = HashSet<String>()
        context.contentResolver.query(children, arrayOf(DocumentsContract.Document.COLUMN_DISPLAY_NAME), null, null, null)?.use { cursor ->
            val column = cursor.getColumnIndex(DocumentsContract.Document.COLUMN_DISPLAY_NAME)
            if (column >= 0) while (cursor.moveToNext()) existing.add(cursor.getString(column))
        }
        if (base !in existing) return base
        val dot = base.lastIndexOf('.').takeIf { it > 0 } ?: base.length
        val stem = base.substring(0, dot)
        val extension = base.substring(dot)
        for (suffix in 1..999) {
            val candidate = "$stem ($suffix)$extension"
            if (candidate !in existing) return candidate
        }
        throw IllegalArgumentException("The receive folder already contains too many files named '$base'.")
    }

    private fun startReceive(id: String, call: PluginCall?) {
        if (!activeReceiveTransfers.add(id)) { call?.reject("This transfer is already being received.", "BUSY"); return }
        transferWorker.execute {
            try { receiveFile(id, call) }
            catch (error: Exception) {
                android.util.Log.e("RipplePlugin", "Incoming transfer failed", error)
                if (call != null) call.reject(error.message ?: "The file could not be received.", "TRANSFER_FAILED")
                else {
                    transferRecords[id]?.let { item ->
                        item.put("state", "failed"); item.put("updatedAt", System.currentTimeMillis())
                        runCatching { saveTransfers() }; changed()
                    }
                }
            } finally { activeReceiveTransfers.remove(id) }
        }
    }
    @PluginMethod fun acceptTransfer(call: PluginCall) {
        val id = call.getString("transferId")
        if (id == null) { call.reject("Transfer ID is required.", "INVALID_REQUEST"); return }
        startReceive(id, call)
    }
    @PluginMethod fun rejectTransfer(call: PluginCall) = task(call) {
        val id = call.getString("transferId") ?: throw IllegalArgumentException("Transfer ID is required.")
        val item = transferRecords[id] ?: throw IllegalArgumentException("Transfer not found.")
        require(item.optString("direction") == "receive" && item.optString("state") == "offered") { "This file is no longer waiting for a decision." }
        val peer = trustedPeers[item.optString("peerId")] ?: throw IllegalArgumentException("This PC is no longer trusted. Pair it again to manage its transfer.")
        val deviceId = prefs.getString("deviceId", null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
        val result = PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId)
            .json("POST", "/v1/transfers/$id/decision", JSONObject().put("approve", false))
        require(result.optString("state") == "rejected") { "The PC did not confirm rejection." }
        item.put("state", "rejected"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed(); call.resolve()
    }
    @PluginMethod fun cancelTransfer(call: PluginCall) = task(call) {
        val id = call.getString("transferId") ?: throw IllegalArgumentException("Transfer ID is required.")
        val item = transferRecords[id] ?: throw IllegalArgumentException("Transfer not found.")
        require(item.optString("state") !in setOf("completed", "rejected", "cancelled", "failed")) { "This transfer has already finished." }
        val direction = item.optString("direction")
        require(direction == "send" || direction == "receive") { "This transfer direction is not supported." }
        val peer = trustedPeers[item.optString("peerId")]
        if (peer == null) {
            val active = if (direction == "send") activeSendTransfers.contains(id) else activeReceiveTransfers.contains(id)
            if (active) cancellationRequests.add(id)
            if (direction == "receive") item.optString("uri").takeIf { it.isNotBlank() }?.let { runCatching { DocumentsContract.deleteDocument(context.contentResolver, Uri.parse(it)) }; item.remove("uri") }
            item.put("state", "cancelled"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
            call.resolve(JSObject().put("remoteConfirmed", false).put("message", "Cancelled on this phone. This PC is no longer trusted, so Ripple could not clear any partial file on it."))
            return@task
        }
        val isActive = if (direction == "send") activeSendTransfers.contains(id) else activeReceiveTransfers.contains(id)
        val deviceId = prefs.getString("deviceId", null) ?: throw IllegalArgumentException("Android identity is unavailable. Pair this device again.")
        cancellationRequests.add(id)
        try {
            val result = PeerTransferClient(peer.getString("endpoint"), peer.getString("fingerprint"), deviceId)
                .json("POST", "/v1/transfers/$id/cancel")
            require(result.optString("state") == "cancelled") { "The PC did not confirm transfer cancellation." }
            if (direction == "receive") item.optString("uri").takeIf { it.isNotBlank() }?.let { runCatching { DocumentsContract.deleteDocument(context.contentResolver, Uri.parse(it)) }; item.remove("uri") }
            item.put("state", "cancelled"); item.put("updatedAt", System.currentTimeMillis()); saveTransfers(); changed()
            call.resolve(JSObject().put("remoteConfirmed", true))
        } catch (error: Exception) {
            if (!isActive) cancellationRequests.remove(id)
            throw error
        }
    }
    @PluginMethod fun resumeTransfer(call: PluginCall) { call.reject("Transfer resume is not available yet.", "UNAVAILABLE") }
    @PluginMethod fun queryTransfer(call: PluginCall) = task(call) { val id=call.getString("transferId")?:throw IllegalArgumentException("Transfer ID is required.");val item=transferRecords[id]?:throw IllegalArgumentException("Transfer not found.");call.resolve(JSObject().put("id",id).put("name",item.optString("name")).put("direction",item.optString("direction")).put("state",item.optString("state")).put("size",item.optLong("size")).put("offset",item.optLong("offset")).put("peerName",item.optString("peerName"))) }
    @PluginMethod fun submitAiRequest(call: PluginCall) { hostUnavailable(call) }
    @PluginMethod fun approveAiAction(call: PluginCall) { hostUnavailable(call) }
    @PluginMethod fun cancelAiRequest(call: PluginCall) { call.reject("AI request not found.", "UNKNOWN_REQUEST") }
    override fun handleOnPause() { monitorEnabled = false; desktopDropActive = false; changed(); stop() }
    override fun handleOnResume() { monitorEnabled = true; transferWorker.execute { pollIncomingOffers() }; if (trustedPeers.isNotEmpty()) activity.runOnUiThread { startDiscoveryInternal(null) } }
    override fun handleOnDestroy() { monitorEnabled = false; stop(); incomingMonitor.shutdownNow(); worker.shutdown(); transferWorker.shutdownNow(); dropWorker.shutdownNow() }
}
