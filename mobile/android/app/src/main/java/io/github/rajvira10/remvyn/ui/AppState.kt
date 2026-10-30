package io.github.rajvira10.remvyn.ui

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.os.PersistableBundle
import android.os.Handler
import android.os.Looper
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.rajvira10.remvyn.ssh.HostKeyQuestion
import io.github.rajvira10.remvyn.ssh.SshCallbacks
import io.github.rajvira10.remvyn.ssh.SshSession
import io.github.rajvira10.remvyn.sync.GitHubException
import io.github.rajvira10.remvyn.sync.Host
import io.github.rajvira10.remvyn.sync.Vault
import io.github.rajvira10.remvyn.sync.VaultStore
import io.github.rajvira10.remvyn.term.Emulator
import io.github.rajvira10.remvyn.term.TerminalView
import java.util.concurrent.CompletableFuture

sealed interface Screen {
    data object Setup : Screen
    /** Picking another repository to add as a vault, with the saved sign-in. */
    data object AddVault : Screen
    data object Unlock : Screen
    data object Hosts : Screen
    data object Passwords : Screen
    data class PasswordItem(val itemId: String) : Screen
    data class Terminal(val hostId: String) : Screen
    data object Settings : Screen
}

private val main = Handler(Looper.getMainLooper())
fun onMain(f: () -> Unit) = main.post(f)

/** One terminal tab: an emulator plus its SSH connection. */
class TermSession(val host: Host, private val app: AppState) : SshCallbacks {
    enum class State { CONNECTING, CONNECTED, CLOSED }

    var state by mutableStateOf(State.CONNECTING)
    var message by mutableStateOf<String?>(null)
    var question by mutableStateOf<HostKeyQuestion?>(null)
    private var answer: CompletableFuture<Boolean>? = null
    private var ssh: SshSession? = null
    var view: TerminalView? = null
    var startedOnce = false
    val emulator = Emulator(80, 24) { reply -> ssh?.write(reply.toByteArray()) }

    fun start(password: String, cols: Int, rows: Int) {
        state = State.CONNECTING
        message = null
        emulator.resize(cols, rows)
        if (app.vault == null) return
        val vault = app.vault!!
        val known = (vault.knownHosts["${host.hostname}:${host.port}"] ?: emptyList()) +
            app.store.trustedHere("${host.hostname}:${host.port}")
        val s = SshSession(host, vault.keys[host.keyId], password.ifEmpty { host.password }, known,
            { k -> app.store.trustHere("${host.hostname}:${host.port}", k) }, this)
        ssh = s
        Thread {
            try {
                s.connect(cols, rows)
                onMain { state = State.CONNECTED; view?.showKeyboard() }
            } catch (e: Exception) {
                val msg = friendly(e)
                onMain { state = State.CLOSED; message = msg }
            }
        }.start()
    }

    private fun friendly(e: Exception): String {
        val m = e.message ?: e.javaClass.simpleName
        return when {
            m.contains("Auth fail", true) || m.contains("Auth cancel", true) -> "Authentication failed. Check the saved password or key on your computer."
            m.contains("reject HostKey", true) -> "Connection cancelled: the server's host key wasn't trusted."
            m.contains("timeout", true) -> "The server didn't answer in time."
            m.contains("UnknownHost", true) || e.cause is java.net.UnknownHostException -> "Couldn't find ${host.hostname}."
            m.contains("Connection refused", true) -> "${host.hostname} refused the connection on port ${host.port}."
            else -> m
        }
    }

    fun write(bytes: ByteArray) = ssh?.write(bytes)
    fun resize(cols: Int, rows: Int) = ssh?.resize(cols, rows)

    fun answerHostKey(ok: Boolean) {
        question = null
        answer?.complete(ok)
    }

    override fun confirmHostKey(q: HostKeyQuestion): Boolean {
        val f = CompletableFuture<Boolean>()
        answer = f
        onMain { question = q }
        return try { f.get() } catch (_: Exception) { false }
    }

    override fun onOutput(data: ByteArray, len: Int) {
        emulator.append(data, len)
        view?.onScreenChanged()
    }

    override fun onClosed(error: String?) {
        onMain {
            if (state != State.CLOSED) {
                state = State.CLOSED
                message = error
            }
        }
    }

    fun close() {
        answer?.complete(false)
        ssh?.close()
        state = State.CLOSED
    }
}

/** How long a copied secret stays on the clipboard, like the desktop. */
const val CLIPBOARD_CLEAR_MS = 30_000L
private const val CLIP_TOKEN = "io.github.rajvira10.remvyn.clip"

class AppState(ctx: Context) {
    val store = VaultStore(ctx.applicationContext)
    private val clipboard = ctx.applicationContext.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    /** Tags the last secret we copied, so we only ever clear our own clip. */
    private var clipToken = 0L
    private val clearClip = Runnable { clearSecretClip() }
    var screen by mutableStateOf<Screen>(if (store.hasVault()) Screen.Unlock else Screen.Setup)
    /** The bottom-bar tab to return to from Settings. */
    var lastTab by mutableStateOf<Screen>(Screen.Hosts)
    var vault by mutableStateOf<Vault?>(null)
    var syncing by mutableStateOf(false)
    var syncError by mutableStateOf<String?>(null)
    var lastSyncAt by mutableStateOf(store.config().lastSyncAt)
    var fontSize by mutableStateOf(ctx.getSharedPreferences("ui", 0).getFloat("fontSize", 13f))
    val sessions = mutableStateMapOf<String, TermSession>()
    /** Password typed for a host without a saved one, used by its next connect. */
    var pendingPassword = ""
    var version by mutableIntStateOf(0)
    private val prefs = ctx.getSharedPreferences("ui", 0)

    /**
     * Copies a secret, marked sensitive so Android 13+ hides it from the
     * clipboard preview, and clears it after [CLIPBOARD_CLEAR_MS] or on lock
     * unless something else was copied since.
     */
    fun copySecret(label: String, text: String) {
        val clip = ClipData.newPlainText(label, text)
        clip.description.extras = PersistableBundle().apply {
            putBoolean(if (Build.VERSION.SDK_INT >= 33) ClipDescription.EXTRA_IS_SENSITIVE else "android.content.extra.IS_SENSITIVE", true)
            clipToken = System.nanoTime()
            putLong(CLIP_TOKEN, clipToken)
        }
        clipboard.setPrimaryClip(clip)
        main.removeCallbacks(clearClip)
        main.postDelayed(clearClip, CLIPBOARD_CLEAR_MS)
    }

    private fun clearSecretClip() {
        main.removeCallbacks(clearClip)
        if (clipToken == 0L) return
        val token = clipToken
        clipToken = 0L
        // Android only lets the focused app read the clipboard. If we can see
        // it and it's no longer our clip, leave it; in the background (no
        // access) clear anyway rather than leave a secret behind.
        val desc = runCatching { clipboard.primaryClipDescription }.getOrNull()
        if (desc != null && desc.extras?.getLong(CLIP_TOKEN) != token) return
        if (Build.VERSION.SDK_INT >= 28) clipboard.clearPrimaryClip() else clipboard.setPrimaryClip(ClipData.newPlainText("", ""))
    }

    fun unlocked() {
        vault = store.vault
        lastSyncAt = store.config().lastSyncAt
        syncError = null
        screen = Screen.Hosts
        sync()
    }

    private fun closeSessions() {
        clearSecretClip()
        sessions.values.forEach { it.close() }
        sessions.clear()
    }

    /** A vault was added and opened; terminals of the previous one close, like switching. */
    fun vaultAdded() {
        closeSessions()
        version++
        unlocked()
    }

    /** Opens another vault on this phone. It starts locked. */
    fun switchVault(id: String) {
        if (id == store.activeId()) return
        closeSessions()
        store.switchTo(id)
        vault = null
        syncError = null
        lastSyncAt = store.config().lastSyncAt
        version++
        screen = Screen.Unlock
    }

    /** Removes a vault from this phone. Removing the open one locks and opens the next; the last one signs out. */
    fun removeVault(id: String) {
        val wasActive = id == store.activeId()
        if (wasActive) closeSessions()
        store.remove(id)
        version++
        when {
            store.profiles().isEmpty() -> signOut()
            wasActive -> {
                vault = null
                syncError = null
                lastSyncAt = store.config().lastSyncAt
                screen = Screen.Unlock
            }
        }
    }

    fun sync() {
        if (syncing || !store.unlocked) return
        syncing = true
        syncError = null
        Thread {
            val result = runCatching { store.sync() }
            onMain {
                syncing = false
                result.onSuccess { if (it) vault = store.vault }
                result.onFailure { syncError = (it as? GitHubException)?.message ?: it.message }
                lastSyncAt = store.config().lastSyncAt
            }
        }.start()
    }

    fun saveFontSize(v: Float) {
        fontSize = v
        prefs.edit().putFloat("fontSize", v).apply()
    }

    fun open(host: Host) {
        screen = Screen.Terminal(host.id)
    }

    fun lock() {
        clearSecretClip()
        sessions.values.forEach { it.close() }
        sessions.clear()
        store.lock()
        vault = null
        screen = Screen.Unlock
    }

    fun signOut() {
        clearSecretClip()
        sessions.values.forEach { it.close() }
        sessions.clear()
        store.erase()
        vault = null
        lastSyncAt = 0
        screen = Screen.Setup
    }
}
