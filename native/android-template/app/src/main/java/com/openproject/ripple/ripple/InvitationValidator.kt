package com.openproject.ripple.ripple

import org.json.JSONObject
import java.net.URI
import java.time.Instant
import java.util.Base64
import java.util.UUID

/** Parses a proposed invitation only; validation never authorizes or enrolls a peer. */
object InvitationValidator {
    const val MAX_BYTES = 4096

    data class Invitation(val endpoint: URI, val fingerprint: String, val expiresAt: Instant, val sessionId: String, val secret: ByteArray)

    fun parse(input: String, now: Instant = Instant.now()): Invitation {
        require(input.toByteArray(Charsets.UTF_8).size <= MAX_BYTES) { "Pairing invitation is too large." }
        val json = try { JSONObject(input) } catch (_: Exception) { throw IllegalArgumentException("Paste a valid Ripple pairing invitation.") }
        require(json.opt("version") is Int && json.getInt("version") == 1) { "Unsupported pairing invitation version." }
        fun field(name: String): String {
            val value = json.opt(name)
            require(value is String && value.isNotBlank()) { "Pairing invitation is missing $name." }
            return value
        }
        val endpoint = try { URI(field("endpoint")) } catch (_: Exception) { throw IllegalArgumentException("The invitation endpoint is invalid.") }
        require(endpoint.scheme == "https" && !endpoint.host.isNullOrBlank() && endpoint.userInfo == null && endpoint.fragment == null && endpoint.query == null && (endpoint.path.isNullOrEmpty() || endpoint.path == "/") && (endpoint.port == -1 || endpoint.port in 1..65535)) { "The invitation must contain an HTTPS host endpoint." }
        require(field("fingerprint").matches(Regex("[a-fA-F0-9]{64}"))) { "The invitation public-key fingerprint must be SHA-256." }
        val expiry = try { Instant.parse(field("expiresAt")) } catch (_: Exception) { throw IllegalArgumentException("The invitation expiry is invalid.") }
        require(expiry.isAfter(now)) { "This invitation has expired. Generate a new code on your PC." }
        require(!expiry.isAfter(now.plusSeconds(120))) { "Pairing invitations must expire within two minutes." }
        val session = field("sessionId")
        require(runCatching { UUID.fromString(session).toString().equals(session, ignoreCase = true) }.getOrDefault(false)) { "The invitation session ID is invalid." }
        val secret = field("secret")
        require(secret.matches(Regex("[A-Za-z0-9_-]{43}"))) { "The invitation secret is invalid." }
        val bytes = try { Base64.getUrlDecoder().decode(secret) } catch (_: Exception) { throw IllegalArgumentException("The invitation secret is invalid.") }
        require(bytes.size == 32 && Base64.getUrlEncoder().withoutPadding().encodeToString(bytes) == secret) { "The invitation secret must contain 256 bits." }
        return Invitation(endpoint, field("fingerprint").lowercase(), expiry, session, bytes)
    }

    fun validate(input: String, now: Instant = Instant.now()) { parse(input, now) }
}
