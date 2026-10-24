package io.github.rajvira10.flux.ui

import android.content.ClipboardManager
import android.content.Context
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import io.github.rajvira10.flux.term.TermColors
import io.github.rajvira10.flux.term.TerminalView

@Composable
fun TerminalScreen(app: AppState, hostId: String) {
    val ctx = LocalContext.current
    val vault = app.vault ?: return
    val host = vault.hosts.find { it.id == hostId }
    if (host == null) {
        app.screen = Screen.Hosts
        return
    }
    val session = app.sessions.getOrPut(hostId) { TermSession(host, app) }
    var menu by remember { mutableStateOf(false) }
    var ctrl by remember { mutableStateOf(false) }
    var alt by remember { mutableStateOf(false) }
    var viewRef by remember { mutableStateOf<TerminalView?>(null) }

    BackHandler { app.screen = Screen.Hosts }

    Column(Modifier.fillMaxSize().background(Color(TermColors.FLUX.background)).statusBarsPadding().navigationBarsPadding().imePadding()) {
        // Toolbar
        Row(Modifier.fillMaxWidth().background(Background).padding(horizontal = 4.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { app.screen = Screen.Hosts }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Hosts") }
            Box(Modifier.size(8.dp).background(when (session.state) {
                TermSession.State.CONNECTED -> Success
                TermSession.State.CONNECTING -> Warning
                TermSession.State.CLOSED -> Muted
            }, CircleShape))
            Spacer(Modifier.size(8.dp))
            Column(Modifier.weight(1f)) {
                Text(host.label, fontWeight = FontWeight.Medium, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text("${host.username.ifEmpty { "root" }}@${host.hostname}", color = Muted, fontSize = 11.sp, fontFamily = FontFamily.Monospace, maxLines = 1)
            }
            if (session.state == TermSession.State.CLOSED) {
                IconButton(onClick = { val (c, r) = viewRef?.gridSize() ?: (80 to 24); session.start(app.pendingPassword, c, r) }) {
                    Icon(Icons.Default.Refresh, "Reconnect")
                }
            }
            Box {
                IconButton(onClick = { menu = true }) { Icon(Icons.Default.MoreVert, "More") }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(text = { Text("Paste") }, onClick = {
                        menu = false
                        val clip = (ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).primaryClip
                        clip?.getItemAt(0)?.coerceToText(ctx)?.toString()?.let { viewRef?.paste(it) }
                    })
                    DropdownMenuItem(text = { Text("Text size +") }, onClick = { viewRef?.let { it.fontSizeSp += 1; app.saveFontSize(it.fontSizeSp) } })
                    DropdownMenuItem(text = { Text("Text size −") }, onClick = { viewRef?.let { it.fontSizeSp -= 1; app.saveFontSize(it.fontSizeSp) } })
                    if (vault.snippets.isNotEmpty()) {
                        HorizontalDivider()
                        Text("Snippets", color = Muted, fontSize = 11.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
                        vault.snippets.forEach { sn ->
                            DropdownMenuItem(
                                text = {
                                    Column {
                                        Text(sn.name)
                                        Text(sn.command, color = Muted, fontSize = 11.sp, fontFamily = FontFamily.Monospace, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
                                },
                                onClick = {
                                    menu = false
                                    val cmd = if (sn.command.endsWith("\n")) sn.command else sn.command + "\r"
                                    session.write(cmd.replace("\n", "\r").toByteArray())
                                },
                                enabled = session.state == TermSession.State.CONNECTED,
                            )
                        }
                    }
                    HorizontalDivider()
                    DropdownMenuItem(
                        text = { Text("Disconnect", color = Danger) },
                        leadingIcon = { Icon(Icons.Default.Close, null, tint = Danger) },
                        onClick = {
                            menu = false
                            session.close()
                            app.sessions.remove(hostId)
                            app.screen = Screen.Hosts
                        },
                    )
                }
            }
        }

        Box(Modifier.weight(1f).fillMaxWidth()) {
            AndroidView(
                modifier = Modifier.fillMaxSize().clipToBounds().padding(horizontal = 4.dp, vertical = 2.dp),
                factory = { c ->
                    TerminalView(c).apply {
                        fontSizeSp = app.fontSize
                        emulator = session.emulator
                        onInput = { bytes -> session.write(bytes) }
                        onResize = { cols, rows -> session.resize(cols, rows) }
                        onFontSize = { app.saveFontSize(it) }
                        onModifiersUsed = { ctrl = false; alt = false }
                        session.view = this
                        viewRef = this
                        post {
                            if (session.state == TermSession.State.CONNECTING && !session.startedOnce) {
                                session.startedOnce = true
                                val (cols, rows) = gridSize()
                                session.start(app.pendingPassword, cols, rows)
                                app.pendingPassword = ""
                            }
                            showKeyboard()
                        }
                    }
                },
                update = { v ->
                    v.ctrlArmed = ctrl
                    v.altArmed = alt
                },
            )
            if (session.state != TermSession.State.CONNECTED) {
                Surface(
                    color = Surface.copy(alpha = 0.96f), shape = RoundedCornerShape(16.dp),
                    modifier = Modifier.align(Alignment.Center).padding(24.dp),
                ) {
                    Column(Modifier.padding(22.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        if (session.state == TermSession.State.CONNECTING) {
                            CircularProgressIndicator(Modifier.size(26.dp))
                            Spacer(Modifier.height(12.dp))
                            Text("Connecting to ${host.label}…")
                        } else {
                            Text(session.message ?: "Disconnected", textAlign = TextAlign.Center)
                            Spacer(Modifier.height(14.dp))
                            Button(onClick = { val (c, r) = viewRef?.gridSize() ?: (80 to 24); session.start("", c, r) }) { Text("Reconnect") }
                        }
                    }
                }
            }
        }

        // Extra keys
        Row(
            Modifier.fillMaxWidth().background(Background).horizontalScroll(rememberScrollState()).padding(horizontal = 4.dp, vertical = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            @Composable
            fun key(label: String, active: Boolean = false, onClick: () -> Unit) {
                Button(
                    onClick = onClick,
                    colors = ButtonDefaults.buttonColors(containerColor = if (active) Primary else SurfaceHigh, contentColor = if (active) Background else Color(0xFFE3E5EC)),
                    shape = RoundedCornerShape(8.dp),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 12.dp, vertical = 0.dp),
                    modifier = Modifier.height(38.dp),
                ) { Text(label, fontSize = 13.sp, fontFamily = FontFamily.Monospace) }
            }
            val v = viewRef
            key("esc") { session.write(byteArrayOf(27)) }
            key("tab") { session.write(byteArrayOf(9)) }
            key("ctrl", ctrl) { ctrl = !ctrl }
            key("alt", alt) { alt = !alt }
            key("←") { v?.sendKey("D") }
            key("↑") { v?.sendKey("A") }
            key("↓") { v?.sendKey("B") }
            key("→") { v?.sendKey("C") }
            key("home") { session.write("\u001b[H".toByteArray()) }
            key("end") { session.write("\u001b[F".toByteArray()) }
            key("pgup") { session.write("\u001b[5~".toByteArray()) }
            key("pgdn") { session.write("\u001b[6~".toByteArray()) }
            for (s in listOf("|", "/", "-", "~", ":", "*")) key(s) { v?.sendText(s) }
        }
    }

    DisposableEffect(hostId) { onDispose { if (session.view === viewRef) session.view = null } }

    session.question?.let { q ->
        AlertDialog(
            onDismissRequest = {},
            title = { Text(if (q.changed) "Host key changed!" else "Trust this server?", color = if (q.changed) Danger else Color.Unspecified) },
            text = {
                Column {
                    Text(
                        if (q.changed) "The key for ${q.hostPort} doesn’t match the one saved in your vault. Someone may be intercepting the connection. Only continue if you know the server was reinstalled."
                        else "First connection to ${q.hostPort} from this phone. Check the fingerprint matches the server.",
                        fontSize = 14.sp,
                    )
                    Spacer(Modifier.height(12.dp))
                    Text(q.type, color = Muted, fontSize = 12.sp)
                    Text(q.fingerprint, fontFamily = FontFamily.Monospace, fontSize = 12.sp)
                }
            },
            confirmButton = {
                TextButton(onClick = { session.answerHostKey(true) }) { Text(if (q.changed) "Connect anyway" else "Trust and connect", color = if (q.changed) Danger else Primary) }
            },
            dismissButton = { TextButton(onClick = { session.answerHostKey(false) }) { Text("Cancel") } },
        )
    }
}
