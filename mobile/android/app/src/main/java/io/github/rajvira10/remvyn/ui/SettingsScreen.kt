package io.github.rajvira10.remvyn.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.rajvira10.remvyn.BuildConfig
import io.github.rajvira10.remvyn.sync.VaultInfo
import androidx.compose.foundation.clickable

@Composable
private fun Card(title: String, content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp).background(Surface, RoundedCornerShape(14.dp)).padding(16.dp)) {
        Text(title, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
        Spacer(Modifier.size(10.dp))
        content()
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(app: AppState) {
    val ctx = LocalContext.current
    val cfg = remember(app.lastSyncAt) { app.store.config() }
    var confirmSignOut by remember { mutableStateOf(false) }
    var confirmRemove by remember { mutableStateOf<VaultInfo?>(null) }
    BackHandler { app.screen = app.lastTab }

    Scaffold(
        containerColor = Background,
        topBar = {
            TopAppBar(
                title = { Text("Settings") },
                navigationIcon = { IconButton(onClick = { app.screen = app.lastTab }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back") } },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Background),
            )
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState())) {
            Card("Sync") {
                Text(cfg.repo, fontFamily = FontFamily.Monospace, fontSize = 13.sp)
                Text("Signed in as ${cfg.login} · synced ${ago(app.lastSyncAt)}", color = Muted, fontSize = 12.sp)
                app.syncError?.let { Text(it, color = Danger, fontSize = 12.sp, modifier = Modifier.padding(top = 6.dp)) }
                Spacer(Modifier.size(12.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    OutlinedButton(onClick = { app.sync() }, enabled = !app.syncing) {
                        if (app.syncing) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp) else Text("Sync now")
                    }
                    Spacer(Modifier.size(8.dp))
                    TextButton(onClick = { openUrl(ctx, "${cfg.web}/${cfg.repo}") }) { Text("Open on GitHub") }
                }
                Spacer(Modifier.size(8.dp))
                Text(
                    "The vault is read-only on the phone: add and edit hosts, keys, snippets and passwords in Remvyn on your computer. " +
                        "Host keys you trust here are remembered on this phone only.",
                    color = Muted, fontSize = 12.sp,
                )
            }
            Card("Vaults") {
                Text(
                    "Each vault syncs to its own repository and has its own master password, like Settings → Vaults on your computer. " +
                        "Opening another vault closes open terminals.",
                    color = Muted, fontSize = 12.sp,
                )
                Spacer(Modifier.size(8.dp))
                app.vaultList.forEach { v ->
                    val open = v.id == app.currentVault?.id
                    Row(
                        Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(vertical = 2.dp)
                            .background(if (open) SurfaceHigh else Background, RoundedCornerShape(10.dp))
                            .clickable(enabled = !open) { app.switchVault(v.id) }.padding(start = 12.dp, top = 4.dp, bottom = 4.dp, end = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(v.repo, fontFamily = FontFamily.Monospace, fontSize = 13.sp, modifier = Modifier.weight(1f))
                        if (open) Text("open", color = Primary, fontSize = 12.sp, modifier = Modifier.padding(end = 8.dp))
                        else TextButton(onClick = { confirmRemove = v }) { Text("Remove", color = Muted, fontSize = 12.sp) }
                    }
                }
                Spacer(Modifier.size(8.dp))
                OutlinedButton(onClick = { app.screen = Screen.Setup(adding = true) }) { Text("Add vault") }
            }
            Card("Terminal") {
                Text("Text size: ${app.fontSize.toInt()}", color = Muted, fontSize = 13.sp)
                Slider(app.fontSize, { app.saveFontSize(it) }, valueRange = 8f..24f, steps = 15)
                Text("Pinch inside a terminal to resize too.", color = Muted, fontSize = 12.sp)
            }
            Card("Security") {
                Text(
                    "The vault stays encrypted on this phone and opens only with your master password. Remvyn locks when it's been in the background for 5 minutes.",
                    color = Muted, fontSize = 12.sp,
                )
                Spacer(Modifier.size(10.dp))
                Row {
                    OutlinedButton(onClick = { app.lock() }) { Text("Lock now") }
                    Spacer(Modifier.size(8.dp))
                    TextButton(onClick = { confirmSignOut = true }) { Text("Sign out", color = Danger) }
                }
            }
            Text("Remvyn ${BuildConfig.VERSION_NAME}", color = Muted, fontSize = 12.sp, modifier = Modifier.padding(20.dp))
        }
    }

    confirmRemove?.let { v ->
        AlertDialog(
            onDismissRequest = { confirmRemove = null },
            title = { Text("Remove this vault from the phone?") },
            text = { Text("${v.repo} is deleted from this phone. It stays in GitHub and on your computer, and you can add it again.") },
            confirmButton = { TextButton(onClick = { confirmRemove = null; app.removeVault(v.id) }) { Text("Remove", color = Danger) } },
            dismissButton = { TextButton(onClick = { confirmRemove = null }) { Text("Cancel") } },
        )
    }

    if (confirmSignOut) {
        AlertDialog(
            onDismissRequest = { confirmSignOut = false },
            title = { Text("Sign out of this phone?") },
            text = {
                Text(
                    if (app.vaultList.size > 1) "All ${app.vaultList.size} vaults are removed from this phone and open sessions are closed. Everything stays in GitHub and on your computer."
                    else "The vault is removed from this phone and open sessions are closed. Everything stays in GitHub and on your computer.",
                )
            },
            confirmButton = { TextButton(onClick = { confirmSignOut = false; app.signOut() }) { Text("Sign out", color = Danger) } },
            dismissButton = { TextButton(onClick = { confirmSignOut = false }) { Text("Cancel") } },
        )
    }
}
