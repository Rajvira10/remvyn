package io.github.rajvira10.remvyn.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.rajvira10.remvyn.sync.Host

fun ago(ms: Long): String {
    if (ms == 0L) return "never"
    val s = (System.currentTimeMillis() - ms) / 1000
    return when {
        s < 60 -> "just now"
        s < 3600 -> "${s / 60}m ago"
        s < 86400 -> "${s / 3600}h ago"
        else -> "${s / 86400}d ago"
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostsScreen(app: AppState) {
    val vault = app.vault ?: return
    var query by remember { mutableStateOf("") }
    var askPassword by remember { mutableStateOf<Host?>(null) }

    val filtered = vault.hosts.filter {
        query.isBlank() || listOf(it.label, it.hostname, it.username).any { f -> f.contains(query, true) }
    }
    val grouped = filtered.groupBy { vault.groups[it.groupId]?.name ?: "" }.toSortedMap(compareBy({ it.isEmpty() }, { it.lowercase() }))

    Scaffold(
        containerColor = Background,
        topBar = {
            TopAppBar(
                title = { Row(verticalAlignment = Alignment.CenterVertically) { Logo(28.dp); Spacer(Modifier.size(10.dp)); Text("Remvyn") } },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Background),
                actions = {
                    IconButton(onClick = { app.sync() }, enabled = !app.syncing) {
                        if (app.syncing) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                        else Icon(Icons.Default.Refresh, "Sync")
                    }
                    IconButton(onClick = { app.lock() }) { Icon(Icons.Default.Lock, "Lock") }
                    IconButton(onClick = { app.screen = Screen.Settings }) { Icon(Icons.Default.Settings, "Settings") }
                },
            )
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            TextField(
                query, { query = it }, placeholder = { Text("Search ${vault.hosts.size} hosts") }, singleLine = true,
                leadingIcon = { Icon(Icons.Default.Search, null) },
                shape = RoundedCornerShape(12.dp),
                colors = TextFieldDefaults.colors(focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                    unfocusedContainerColor = Surface, focusedContainerColor = SurfaceHigh),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            )
            app.syncError?.let {
                Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp).background(Danger.copy(alpha = 0.12f), RoundedCornerShape(10.dp)).padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Default.Warning, null, tint = Danger, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.size(8.dp))
                    Text(it, color = Danger, fontSize = 13.sp)
                }
            }
            if (vault.hosts.isEmpty()) {
                Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
                    Text("No hosts in your vault yet. Add them in Remvyn on your computer; they appear here after sync.", color = Muted, textAlign = TextAlign.Center)
                }
                return@Column
            }
            LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                grouped.forEach { (group, hosts) ->
                    item(key = "g:$group") {
                        Text(group.ifEmpty { if (grouped.size > 1) "Other" else "Hosts" }.uppercase(), color = Muted, fontSize = 11.sp,
                            fontWeight = FontWeight.Medium, letterSpacing = 1.sp, modifier = Modifier.padding(start = 20.dp, top = 14.dp, bottom = 4.dp))
                    }
                    items(hosts, key = { it.id }) { h ->
                        val session = app.sessions[h.id]
                        Row(
                            Modifier.fillMaxWidth().padding(horizontal = 12.dp).background(Surface, RoundedCornerShape(12.dp))
                                .clickable {
                                    if (session == null && h.authMethod == "password" && h.password.isEmpty()) askPassword = h
                                    else app.open(h)
                                }
                                .padding(horizontal = 12.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Initials(h.label, parseColor(h.color) ?: Primary)
                            Spacer(Modifier.size(12.dp))
                            Column(Modifier.weight(1f)) {
                                Text(h.label, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                Text(
                                    "${h.username.ifEmpty { "root" }}@${h.hostname}${if (h.port != 22) ":${h.port}" else ""}",
                                    color = Muted, fontSize = 12.sp, fontFamily = FontFamily.Monospace, maxLines = 1, overflow = TextOverflow.Ellipsis,
                                )
                            }
                            if (session != null) {
                                Box(Modifier.size(8.dp).background(if (session.state == TermSession.State.CONNECTED) Success else Muted, CircleShape))
                            }
                        }
                    }
                }
                item { Spacer(Modifier.height(12.dp)) }
            }
            Text(
                "Synced ${ago(app.lastSyncAt)} · edit hosts in Remvyn on your computer",
                color = Muted, fontSize = 11.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(10.dp),
            )
        }
    }

    askPassword?.let { h ->
        var pw by remember(h.id) { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { askPassword = null },
            title = { Text("Password for ${h.label}") },
            text = {
                OutlinedTextField(pw, { pw = it }, singleLine = true, label = { Text("Password") },
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password))
            },
            confirmButton = {
                TextButton(onClick = {
                    askPassword = null
                    app.pendingPassword = pw
                    app.open(h)
                }, enabled = pw.isNotEmpty()) { Text("Connect") }
            },
            dismissButton = { TextButton(onClick = { askPassword = null }) { Text("Cancel") } },
        )
    }
}
