package com.openproject.ripple.ripple

import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.security.cert.X509Certificate
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager
import org.conscrypt.Conscrypt

/** Pairing-only HTTPS client. It accepts a server certificate only when its SPKI pin matches the QR. */
internal class PinnedEnrollmentClient(private val invitation: InvitationValidator.Invitation) {
    fun post(path: String, body: JSONObject): JSONObject {
        require(path.startsWith("/v1/enrollment/")) { "Invalid enrollment route." }
        val endpoint = invitation.endpoint.resolve(path)
        require(endpoint.scheme == "https" && endpoint.host == invitation.endpoint.host && endpoint.port == invitation.endpoint.port) {
            "Enrollment endpoint escaped the scanned server address."
        }
        val connection = URL(endpoint.toASCIIString()).openConnection() as HttpsURLConnection
        connection.sslSocketFactory = pinnedContext().socketFactory
        connection.hostnameVerifier = javax.net.ssl.HostnameVerifier { host, session ->
            HttpsURLConnection.getDefaultHostnameVerifier().verify(host, session)
        }
        connection.requestMethod = "POST"
        connection.connectTimeout = 8000
        connection.readTimeout = 8000
        connection.doOutput = true
        connection.setRequestProperty("Content-Type", "application/json")
        connection.setRequestProperty("Accept", "application/json")
        connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
        val status = connection.responseCode
        val stream = if (status in 200..299) connection.inputStream else connection.errorStream
        val response = stream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
        connection.disconnect()
        if (status !in 200..299) throw IllegalStateException("Enrollment server returned HTTP $status.")
        require(response.toByteArray(Charsets.UTF_8).size <= 65536) { "Enrollment response is too large." }
        return JSONObject(response)
    }

    private fun pinnedContext(): SSLContext {
        val manager = object : X509TrustManager {
            override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) {
                throw java.security.cert.CertificateException("Client certificates are not used during enrollment.")
            }
            override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) {
                if (chain.isEmpty()) throw java.security.cert.CertificateException("Server did not present a certificate.")
                val digest = MessageDigest.getInstance("SHA-256").digest(chain[0].publicKey.encoded)
                val actual = digest.joinToString("") { "%02x".format(it) }
                if (!MessageDigest.isEqual(actual.toByteArray(), invitation.fingerprint.toByteArray())) {
                    throw java.security.cert.CertificateException("The server fingerprint does not match the scanned invitation.")
                }
            }
        }
        // Some Android providers expose TLS 1.3 through the generic TLS context only.
        // The Linux server still requires TLS 1.3, so negotiation cannot downgrade there.
        return SSLContext.getInstance("TLS", Conscrypt.newProvider()).apply { init(null, arrayOf<TrustManager>(manager), null) }
    }
}
