package io.github.rajvira10.flux.ssh

import com.jcraft.jsch.ChannelShell
import com.jcraft.jsch.HostKey as JschHostKey
import com.jcraft.jsch.HostKeyRepository
import com.jcraft.jsch.JSch
import com.jcraft.jsch.JSchException
import com.jcraft.jsch.Session
import com.jcraft.jsch.UserInfo
import io.github.rajvira10.flux.sync.Host
import io.github.rajvira10.flux.sync.HostKey
import io.github.rajvira10.flux.sync.SshKey
import java.io.InputStream
import java.io.OutputStream
import java.security.MessageDigest
import java.util.Base64

/** What the UI is asked when a host key isn't trusted yet (or changed). */
data class HostKeyQuestion(val hostPort: String, val type: String, val fingerprint: String, val changed: Boolean)

interface SshCallbacks {
    /** Blocks the connecting thread until the user answers. */
    fun confirmHostKey(q: HostKeyQuestion): Boolean
    fun onOutput(data: ByteArray, len: Int)
    fun onClosed(error: String?)
}

fun fingerprint(key: ByteArray): String =
    "SHA256:" + Base64.getEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(key))

/**
 * One SSH shell. Host keys are checked against the vault's known hosts
 * (from the desktop) plus keys trusted on this phone; a changed key is
 * rejected unless the user explicitly accepts it.
 */
class SshSession(
    private val host: Host,
    private val key: SshKey?,
    private val password: String,
    private val known: List<HostKey>,
    private val trustHere: (HostKey) -> Unit,
    private val cb: SshCallbacks,
) {
    private var session: Session? = null
    private var channel: ChannelShell? = null
    private var out: OutputStream? = null
    @Volatile private var closing = false
    /** One writer thread keeps keystrokes in order. */
    private val writer = java.util.concurrent.Executors.newSingleThreadExecutor()

    val hostPort = "${host.hostname}:${host.port}"

    private inner class Repo : HostKeyRepository {
        override fun check(h: String?, keyBytes: ByteArray): Int {
            if (known.any { it.key.contentEquals(keyBytes) }) return HostKeyRepository.OK
            val type = keyType(keyBytes)
            val ok = cb.confirmHostKey(HostKeyQuestion(hostPort, type, fingerprint(keyBytes), known.isNotEmpty()))
            if (!ok) return HostKeyRepository.NOT_INCLUDED // StrictHostKeyChecking=yes → rejected
            trustHere(HostKey(type, keyBytes))
            return HostKeyRepository.OK
        }

        override fun add(hostkey: JschHostKey?, ui: UserInfo?) {}
        override fun remove(host: String?, type: String?) {}
        override fun remove(host: String?, type: String?, key: ByteArray?) {}
        override fun getKnownHostsRepositoryID() = "flux"
        override fun getHostKey(): Array<JschHostKey> = emptyArray()
        override fun getHostKey(host: String?, type: String?): Array<JschHostKey> = emptyArray()
    }

    /** Connects and starts the shell. Call on a background thread. */
    fun connect(cols: Int, rows: Int) {
        val jsch = JSch()
        jsch.hostKeyRepository = Repo()
        if (host.authMethod == "key") {
            val k = key ?: throw JSchException("The key for this host isn't in the vault")
            val pass = k.passphrase.takeIf { it.isNotEmpty() }?.toByteArray()
            jsch.addIdentity(k.name.ifEmpty { "key" }, k.privateKey.toByteArray(), null, pass)
        } else if (host.authMethod == "agent") {
            throw JSchException("This host uses the SSH agent, which isn't available on the phone. Save a password or key for it on your computer.")
        }
        val s = jsch.getSession(host.username.ifEmpty { "root" }, host.hostname, host.port)
        s.setConfig("StrictHostKeyChecking", "yes")
        s.setConfig("PreferredAuthentications", if (host.authMethod == "key") "publickey" else "password,keyboard-interactive")
        if (password.isNotEmpty()) s.setPassword(password)
        s.userInfo = object : UserInfo {
            override fun getPassphrase(): String? = null
            override fun getPassword(): String = password
            override fun promptPassword(message: String?) = password.isNotEmpty()
            override fun promptPassphrase(message: String?) = false
            override fun promptYesNo(message: String?) = false
            override fun showMessage(message: String?) {}
        }
        s.serverAliveInterval = 30_000
        s.serverAliveCountMax = 3
        s.connect(20_000)
        session = s
        val ch = s.openChannel("shell") as ChannelShell
        ch.setPtyType("xterm-256color", cols, rows, 0, 0)
        ch.setEnv("LANG", "en_US.UTF-8")
        val input: InputStream = ch.inputStream
        out = ch.outputStream
        ch.connect(15_000)
        channel = ch
        if (host.startupCommand.isNotBlank()) write((host.startupCommand + "\r").toByteArray())
        Thread({ pump(input) }, "ssh-${host.label}").start()
    }

    private fun pump(input: InputStream) {
        val buf = ByteArray(16 * 1024)
        var error: String? = null
        try {
            while (true) {
                val n = input.read(buf)
                if (n < 0) break
                if (n > 0) cb.onOutput(buf, n)
            }
        } catch (e: Exception) {
            if (!closing) error = e.message
        }
        val status = channel?.exitStatus ?: -1
        close()
        cb.onClosed(error ?: if (status >= 0) "Session ended (status $status)" else "Session ended")
    }

    fun write(data: ByteArray) {
        val o = out ?: return
        if (writer.isShutdown) return
        writer.execute {
            try {
                o.write(data)
                o.flush()
            } catch (_: Exception) {
            }
        }
    }

    fun resize(cols: Int, rows: Int) {
        if (writer.isShutdown) return
        writer.execute { runCatching { channel?.setPtySize(cols, rows, 0, 0) } }
    }

    val connected get() = channel?.isConnected == true

    fun close() {
        closing = true
        writer.shutdown()
        runCatching { channel?.disconnect() }
        runCatching { session?.disconnect() }
    }

    companion object {
        fun keyType(blob: ByteArray): String {
            if (blob.size < 4) return "unknown"
            val len = ((blob[0].toInt() and 0xff) shl 24) or ((blob[1].toInt() and 0xff) shl 16) or
                ((blob[2].toInt() and 0xff) shl 8) or (blob[3].toInt() and 0xff)
            return if (len in 1..64 && blob.size >= 4 + len) String(blob, 4, len) else "unknown"
        }
    }
}
