package io.github.rajvira10.remvyn.ui

import android.content.Context
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
    data object Unlock : Screen
    data object Hosts : Screen
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

class AppState(ctx: Context) {
    val store = VaultStore(ctx.applicationContext)
    var screen by mutableStateOf<Screen>(if (store.hasVault()) Screen.Unlock else Screen.Setup)
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
        sessions.values.forEach { it.close() }
        sessions.clear()
        store.lock()
        vault = null
        screen = Screen.Unlock
    }

    fun signOut() {
        sessions.values.forEach { it.close() }
        sessions.clear()
        store.erase()
        vault = null
        lastSyncAt = 0
        screen = Screen.Setup
    }
}
