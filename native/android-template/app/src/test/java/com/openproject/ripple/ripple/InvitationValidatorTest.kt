package com.openproject.ripple.ripple

import org.json.JSONObject
import org.junit.Assert.assertThrows
import org.junit.Test
import java.time.Instant
import java.util.Base64

class InvitationValidatorTest {
    private val now = Instant.parse("2026-11-03T00:00:00Z")
    private fun valid() = JSONObject().apply {
        put("version", 1); put("endpoint", "https://192.168.1.4:7843")
        put("fingerprint", "ab".repeat(32)); put("expiresAt", now.plusSeconds(90).toString())
        put("sessionId", "c6fab3e7-a9bd-4f86-b492-022c8cb3f1ba")
        put("secret", Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { it.toByte() }))
    }
    private fun rejects(json: JSONObject) { assertThrows(IllegalArgumentException::class.java) { InvitationValidator.validate(json.toString(), now) } }
    @Test fun acceptsWellFormedInvitationWithoutProducingTrustState() { InvitationValidator.validate(valid().toString(), now) }
    @Test fun rejectsMalformedJson() { assertThrows(IllegalArgumentException::class.java) { InvitationValidator.validate("not-json", now) } }
    @Test fun rejectsOversizedInput() { assertThrows(IllegalArgumentException::class.java) { InvitationValidator.validate(" ".repeat(4097), now) } }
    @Test fun rejectsUnsupportedOrCoercedVersion() { rejects(valid().put("version", 2)); rejects(valid().put("version", "1")); assertThrows(IllegalArgumentException::class.java) { InvitationValidator.validate(valid().toString().replace("\"version\":1", "\"version\":1.0"), now) } }
    @Test fun rejectsExpiredAndExcessiveLifetime() { rejects(valid().put("expiresAt", now.toString())); rejects(valid().put("expiresAt", now.plusSeconds(121).toString())); rejects(valid().put("expiresAt", 0)) }
    @Test fun rejectsUnpinnedOrMalformedFingerprint() { rejects(valid().put("fingerprint", "ab")); rejects(valid().put("fingerprint", "z".repeat(64))) }
    @Test fun rejectsUnsafeEndpoints() { listOf("http://host", "https://user:pass@host", "https://host/path", "https://host?token=1", "https://host#x", "https://host:0", "https://host:70000", "file:///etc/passwd").forEach { rejects(valid().put("endpoint", it)) } }
    @Test fun rejectsMissingOrWrongTypedFields() { rejects(valid().put("endpoint", 1)); rejects(valid().put("secret", JSONObject.NULL)); rejects(valid().apply { remove("sessionId") }) }
    @Test fun rejectsWrongSecretLengthOrEncoding() { rejects(valid().put("secret", "a".repeat(42))); rejects(valid().put("secret", "*".repeat(43))); rejects(valid().put("secret", "_".repeat(43))) }
    @Test fun rejectsNonCanonicalSession() { rejects(valid().put("sessionId", "1-1-1-1-1")) }
}
