package com.openproject.ripple.ripple

import android.util.Base64
import org.json.JSONObject
import java.net.URL
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Signature
import java.security.cert.X509Certificate
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager
import org.conscrypt.Conscrypt

/** Pinned HTTPS client. Every request proves possession of the paired Android key. */
internal class PeerTransferClient(private val endpoint: String, private val fingerprint: String, private val deviceId: String) {
    fun json(method: String, path: String, body: JSONObject? = null): JSONObject {
        val bytes = body?.toString()?.toByteArray(Charsets.UTF_8) ?: ByteArray(0)
        val response = request(method, path, bytes, "application/json")
        require(response.size <= 65536) { "The PC returned an oversized response." }
        return JSONObject(response.toString(Charsets.UTF_8))
    }

    fun checkOnline() {
        val response = request("GET", "/v1/peers/status", ByteArray(0), "application/json")
        require(response.size <= 65536) { "The PC returned an oversized status response." }
        val status = JSONObject(response.toString(Charsets.UTF_8))
        require(status.optInt("version") == 1 && status.optString("state") == "online") {
            "The PC did not confirm its online status."
        }
    }

    fun putChunk(path: String, bytes: ByteArray): JSONObject {
        val response = request("PUT", path, bytes, "application/octet-stream")
        require(response.size <= 65536) { "The PC returned an oversized response." }
        return JSONObject(response.toString(Charsets.UTF_8))
    }

    fun getChunk(path: String): ByteArray {
        val response = request("GET", path, ByteArray(0), "application/octet-stream")
        require(response.size <= (1 shl 20)) { "The PC returned an oversized file chunk." }
        return response
    }

    private fun request(method: String, path: String, body: ByteArray, contentType: String): ByteArray {
        val parts = path.split('/').filter { it.isNotEmpty() }
        val validId = parts.getOrNull(2)?.matches(Regex("[0-9a-fA-F-]{36}")) == true
        val validRoute = when {
            method == "POST" && path == "/v1/transfers" -> true
            method == "GET" && path == "/v1/transfers/inbox" -> true
            parts.size == 3 && parts[0] == "v1" && parts[1] == "transfers" && validId && method == "GET" -> true
            parts.size == 5 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3] == "chunks" && parts[4].toLongOrNull() != null && method == "PUT" -> true
            parts.size == 4 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3] == "complete" && method == "POST" -> true
            parts.size == 4 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3] == "cancel" && method == "POST" -> true
            parts.size == 4 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3] == "decision" && method == "POST" -> true
            parts.size == 4 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3] == "receipt" && method == "POST" -> true
            parts.size == 4 && parts[0] == "v1" && parts[1] == "transfers" && validId && parts[3].startsWith("content?offset=") && method == "GET" -> true
            method == "GET" && path == "/v1/peers/status" -> true
            method == "POST" && path == "/v1/drop-state" -> true
            else -> false
        }
        require(validRoute) { "Invalid transfer route." }
        val base = URL(endpoint)
        val target = URL(base, path)
        require(target.protocol == "https" && target.host == base.host && target.port == base.port) { "Transfer endpoint escaped the trusted PC." }
        val digest = MessageDigest.getInstance("SHA-256").digest(body).joinToString("") { "%02x".format(it) }
        val signature = Signature.getInstance("SHA256withECDSA")
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        signature.initSign(store.getKey("ripple.identity.v1", null) as java.security.PrivateKey)
        signature.update("$method\n$path\n$digest".toByteArray(Charsets.UTF_8))
        val connection = target.openConnection() as HttpsURLConnection
        connection.sslSocketFactory = pinnedContext().socketFactory
        // TLS identity is the QR-pinned SPKI checked by pinnedContext. A LAN address can change while the key stays the same.
        connection.hostnameVerifier = javax.net.ssl.HostnameVerifier { _, _ -> true }
        connection.requestMethod = method
        connection.connectTimeout = 8000
        connection.readTimeout = 15000
        connection.doOutput = body.isNotEmpty()
        connection.setRequestProperty("Accept", "application/json")
        connection.setRequestProperty("Content-Type", contentType)
        connection.setRequestProperty("X-Ripple-Device-ID", deviceId)
        connection.setRequestProperty("X-Ripple-Signature", Base64.encodeToString(signature.sign(), Base64.NO_WRAP))
        if (body.isNotEmpty()) connection.outputStream.use { output ->
            var offset = 0
            while (offset < body.size) {
                val count = minOf(64 * 1024, body.size - offset)
                output.write(body, offset, count)
                offset += count
            }
        }
        val code = connection.responseCode
        val stream = if (code in 200..299) connection.inputStream else connection.errorStream
        val result = stream?.use { input -> input.readBytes() } ?: ByteArray(0)
        connection.disconnect()
        if (code !in 200..299) throw IllegalStateException("PC transfer request failed (HTTP $code).")
        return result
    }

    private fun pinnedContext(): SSLContext {
        val trust = object : X509TrustManager {
            override fun getAcceptedIssuers() = emptyArray<X509Certificate>()
            override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) { throw java.security.cert.CertificateException("Unexpected client certificate") }
            override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) {
                if (chain.isEmpty()) throw java.security.cert.CertificateException("PC did not present a certificate")
                val actual = MessageDigest.getInstance("SHA-256").digest(chain[0].publicKey.encoded).joinToString("") { "%02x".format(it) }
                if (!MessageDigest.isEqual(actual.toByteArray(), fingerprint.toByteArray())) throw java.security.cert.CertificateException("PC identity changed; pair it again to continue")
            }
        }
        return SSLContext.getInstance("TLS", Conscrypt.newProvider()).apply { init(null, arrayOf<TrustManager>(trust), null) }
    }
}
