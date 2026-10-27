package io.github.rajvira10.remvyn

import io.github.rajvira10.remvyn.crypto.RecordCipher
import io.github.rajvira10.remvyn.crypto.Totp
import io.github.rajvira10.remvyn.crypto.VaultCrypto
import io.github.rajvira10.remvyn.sync.VaultItem
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/** Password vault items written by the desktop, and two-factor codes. */
class PasswordsTest {
    private val fixture = JSONObject(javaClass.getResource("/vault-fixture.json")!!.readText())

    @Test
    fun readsDesktopVaultItem() {
        val c = RecordCipher(VaultCrypto.unwrapSyncKey(fixture.getString("vault"), "correct horse battery"))
        assertEquals(fixture.getString("itemPath"), c.path("vaultitem", "v1"))
        val p = c.open(fixture.getString("itemPath"), fixture.getString("itemContent"))
        assertEquals("vaultitem", p.kind)
        val item = VaultItem.fromRecord(p.id, p.data)
        assertEquals("GitHub", item.name)
        assertEquals("login", item.kind)
        assertEquals("octo", item.username)
        assertEquals("s3cr€t", item.password)
        assertTrue(item.favorite)
        assertEquals(1700000000000L, item.updatedAt)
        assertEquals(1, item.fields.size)
        assertEquals("abcd-efgh", item.fields[0].value)
        assertTrue(item.fields[0].hidden)
        // The RFC 6238 SHA1 test key, so the code is known.
        assertEquals("287082", Totp.parse(item.totp).at(59_000).first)
    }

    // RFC 6238 appendix B, 8 digits.
    @Test
    fun rfc6238Vectors() {
        val keys = mapOf(
            "SHA1" to "12345678901234567890",
            "SHA256" to "12345678901234567890123456789012",
            "SHA512" to "1234567890123456789012345678901234567890123456789012345678901234",
        )
        val cases = listOf(
            Triple(59L, "SHA1", "94287082"), Triple(59L, "SHA256", "46119246"), Triple(59L, "SHA512", "90693936"),
            Triple(1111111109L, "SHA1", "07081804"), Triple(1111111109L, "SHA256", "68084774"), Triple(1111111109L, "SHA512", "25091201"),
            Triple(2000000000L, "SHA1", "69279037"), Triple(20000000000L, "SHA256", "77737706"), Triple(20000000000L, "SHA512", "47863826"),
        )
        for ((t, alg, want) in cases) {
            assertEquals("$alg @ $t", want, Totp(keys.getValue(alg).toByteArray(), alg, 8).at(t * 1000).first)
        }
    }

    @Test
    fun parsesSecretsAndLinks() {
        val plain = Totp.parse("gezd gnbv gy3t qojq gezd gnbv gy3t qojq")
        assertEquals("12345678901234567890", String(plain.secret))
        assertEquals(30 - 59 % 30, plain.at(59_000).second)
        val link = Totp.parse("otpauth://totp/My Site:me@x.com?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&digits=8&period=60&algorithm=sha256")
        assertEquals("SHA256", link.algorithm)
        assertEquals(8, link.digits)
        assertEquals(60, link.period)
        assertThrows(IllegalArgumentException::class.java) { Totp.parse("otpauth://hotp/x?secret=GEZDGNBV") }
        assertThrows(IllegalArgumentException::class.java) { Totp.parse("not base32!") }
        assertThrows(IllegalArgumentException::class.java) { Totp.parse("") }
    }
}
