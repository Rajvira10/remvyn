package io.github.rajvira10.flux

import io.github.rajvira10.flux.crypto.RecordCipher
import io.github.rajvira10.flux.crypto.VaultCrypto
import io.github.rajvira10.flux.crypto.WrongPasswordException
import io.github.rajvira10.flux.term.Emulator
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

/** Decrypts a vault written by the desktop's Go code (backend/vaultsync/fixture_test.go). */
class VaultCompatTest {
    private val fixture = JSONObject(javaClass.getResource("/vault-fixture.json")!!.readText())

    @Test
    fun opensDesktopVault() {
        val key = VaultCrypto.unwrapSyncKey(fixture.getString("vault"), "correct horse battery")
        val c = RecordCipher(key)
        assertEquals(fixture.getString("keyId"), c.keyId)
        assertEquals(fixture.getString("path"), c.path("host", "h1"))
        val p = c.open(fixture.getString("path"), fixture.getString("content"))
        assertEquals("host", p.kind)
        assertEquals(42L, p.changedAt)
        assertEquals("prod <db>", p.data.getString("label"))
        assertEquals("pä\$\$wörd", p.data.getString("password"))
        assertEquals(2222, p.data.getInt("port"))
    }

    @Test
    fun wrongPasswordIsRejected() {
        assertThrows(WrongPasswordException::class.java) { VaultCrypto.unwrapSyncKey(fixture.getString("vault"), "nope") }
    }

    @Test
    fun recordBoundToItsPath() {
        val c = RecordCipher(VaultCrypto.unwrapSyncKey(fixture.getString("vault"), "correct horse battery"))
        assertThrows(Exception::class.java) { c.open("records/00000000000000000000000000000000", fixture.getString("content")) }
    }

    private fun Emulator.text(row: Int) = read { String(screen[row].chars, 0, cols).trimEnd() }

    @Test
    fun emulatorBasics() {
        val replies = StringBuilder()
        val e = Emulator(20, 5) { replies.append(it) }
        fun feed(s: String) = s.toByteArray().let { e.append(it, it.size) }
        feed("hello\r\nworld")
        assertEquals("hello", e.text(0))
        assertEquals("world", e.text(1))
        feed("\u001b[2J\u001b[3;5Hx\u001b[6n")
        assertEquals("    x", e.text(2))
        assertEquals("\u001b[3;6R", replies.toString())
        // Wrapping and scrolling into scrollback.
        feed("\u001b[H\u001b[2J" + "a".repeat(25) + "\r\n1\r\n2\r\n3\r\n4")
        assertEquals("4", e.text(4))
        assertEquals(1, e.read { scrollback.size })
        // Colors, then the alternate screen restores the main one.
        feed("\u001b[31;1mR\u001b[0m")
        assertEquals(2L, Emulator.fg(e.read { screen[4].styles[1] }))
        feed("\u001b[?1049h\u001b[Halt")
        assertEquals("alt", e.text(0))
        feed("\u001b[?1049l")
        assertEquals("4R", e.text(4))
        // UTF-8 split across reads.
        val bytes = "é→".toByteArray()
        e.append(bytes.copyOfRange(0, 1), 1)
        e.append(bytes.copyOfRange(1, bytes.size), bytes.size - 1)
        assertEquals("4Ré→", e.text(4))
    }
}
