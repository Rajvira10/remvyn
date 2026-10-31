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
import io.github.rajvira10.remvyn.sync.VaultInfo
import io.github.rajvira10.remvyn.sync.VaultStore
import io.github.rajvira10.remvyn.sync.Vaults
import io.github.rajvira10.remvyn.term.Emulator
import io.github.rajvira10.remvyn.term.TerminalView
import java.util.concurrent.CompletableFuture

sealed interface Screen {
    /** Sign-in and first download; adding is true when adding another vault. */
    data class Setup(val adding: Boolean = false) : Screen
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
    val vaults = Vaults(ctx.applicationContext).apply { cleanUp() }
    /** The open (or locked) vault. */
    var store by mutableStateOf(vaults.store(vaults.current))
        private set
    /** Vaults on this phone, refreshed when one is added or removed. */
    var vaultList by mutableStateOf(vaults.list())
        private set
    /** Id of the open (or locked) vault. */
    var currentId by mutableStateOf(vaults.current)
        private set
    private val clipboard = ctx.applicationContext.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    /** Tags the last secret we copied, so we only ever clear our own clip. */
    private var clipToken = 0L
    private val clearClip = Runnable { clearSecretClip() }
    var screen by mutableStateOf<Screen>(Screen.Setup())
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

    init {
        // The last vault opened may have been removed.
        if (!store.hasVault()) vaultList.firstOrNull()?.let { useVault(it.id) }
        screen = if (store.hasVault()) Screen.Unlock else Screen.Setup()
    }

    val currentVault: VaultInfo? get() = vaultList.firstOrNull { it.id == currentId }

    private fun useVault(id: String) {
        vaults.current = id
        currentId = id
        store = vaults.store(id)
        lastSyncAt = store.config().lastSyncAt
        syncError = null
    }

    private fun closeAll() {
        clearSecretClip()
        sessions.values.forEach { it.close() }
        sessions.clear()
        store.lock()
        vault = null
    }

    /** Locks the open vault and asks for another one's master password. */
    fun switchVault(id: String) {
        if (id == currentId && vault != null) return
        closeAll()
        useVault(id)
        screen = Screen.Unlock
    }

    /** A vault just downloaded by the setup screen into [added]; it becomes the open one. */
    fun vaultAdded(added: VaultStore, id: String) {
        if (store !== added) closeAll()
        vaults.current = id
        currentId = id
        store = added
        lastSyncAt = added.config().lastSyncAt
        syncError = null
        vaultList = vaults.list()
        unlocked()
    }

    /** Deletes a vault from this phone; removing the open one moves to another, or to setup. */
    fun removeVault(id: String) {
        if (id == currentId) {
            closeAll()
            store.erase()
            vaultList = vaults.list()
            val next = vaultList.firstOrNull()
            if (next != null) useVault(next.id) else useVault(Vaults.DEFAULT)
            screen = if (next != null) Screen.Unlock else Screen.Setup()
        } else {
            vaults.remove(id)
            vaultList = vaults.list()
        }
    }

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
        screen = Screen.Hosts
        sync()
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
        closeAll()
        screen = Screen.Unlock
    }

    /** Signs out of GitHub: every vault is removed from this phone. */
    fun signOut() {
        closeAll()
        vaultList.forEach { vaults.remove(it.id) }
        vaults.cleanUp()
        vaultList = vaults.list()
        useVault(Vaults.DEFAULT)
        lastSyncAt = 0
        screen = Screen.Setup()
    }
}
