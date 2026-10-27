package io.github.rajvira10.remvyn.ui

import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Password
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.StickyNote2
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.rajvira10.remvyn.crypto.Totp
import io.github.rajvira10.remvyn.sync.VaultItem
import kotlinx.coroutines.delay

/** Bottom bar shared by the Hosts and Passwords screens. */
@Composable
fun MainTabs(app: AppState) {
    NavigationBar(containerColor = Surface) {
        NavigationBarItem(
            selected = app.screen == Screen.Hosts, onClick = { app.screen = Screen.Hosts; app.lastTab = Screen.Hosts },
            icon = { Icon(Icons.Default.Dns, null) }, label = { Text("Hosts") },
        )
        NavigationBarItem(
            selected = app.screen == Screen.Passwords, onClick = { app.screen = Screen.Passwords; app.lastTab = Screen.Passwords },
            icon = { Icon(Icons.Default.Password, null) }, label = { Text("Passwords") },
        )
    }
}

private fun kindIcon(kind: String): ImageVector = when (kind) {
    "token" -> Icons.Default.Key
    "note" -> Icons.Default.StickyNote2
    else -> Icons.Default.Language
}

private val kinds = listOf("" to "All", "login" to "Logins", "token" to "API tokens", "note" to "Notes")

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PasswordsScreen(app: AppState) {
    val vault = app.vault ?: return
    var query by remember { mutableStateOf("") }
    var kind by remember { mutableStateOf("") }
    BackHandler { app.screen = Screen.Hosts; app.lastTab = Screen.Hosts }

    val filtered = vault.items.filter {
        (kind.isEmpty() || it.kind == kind) &&
            (query.isBlank() || listOf(it.name, it.username, it.url).any { f -> f.contains(query, true) })
    }

    Scaffold(
        containerColor = Background,
        topBar = {
            TopAppBar(
                title = { Text("Passwords") },
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
        bottomBar = { MainTabs(app) },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            TextField(
                query, { query = it }, placeholder = { Text("Search ${vault.items.size} items") }, singleLine = true,
                leadingIcon = { Icon(Icons.Default.Search, null) },
                shape = RoundedCornerShape(12.dp),
                colors = TextFieldDefaults.colors(focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                    unfocusedContainerColor = Surface, focusedContainerColor = SurfaceHigh),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            )
            Row(Modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                kinds.forEach { (k, label) ->
                    FilterChip(kind == k, { kind = k }, { Text(label, fontSize = 12.sp) },
                        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = Primary.copy(alpha = 0.2f)))
                }
            }
            if (vault.items.isEmpty()) {
                Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
                    Text("No passwords yet. Add logins, API tokens and notes under Passwords in Remvyn on your computer; they appear here after sync.",
                        color = Muted, textAlign = TextAlign.Center)
                }
                return@Column
            }
            LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                items(filtered, key = { it.id }) { item ->
                    Row(
                        Modifier.fillMaxWidth().padding(horizontal = 12.dp).background(Surface, RoundedCornerShape(12.dp))
                            .clickable { app.screen = Screen.PasswordItem(item.id) }
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(Modifier.size(36.dp).background(SurfaceHigh, RoundedCornerShape(10.dp)), contentAlignment = Alignment.Center) {
                            Icon(kindIcon(item.kind), null, tint = Primary, modifier = Modifier.size(18.dp))
                        }
                        Spacer(Modifier.size(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(item.name.ifEmpty { "Untitled" }, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            val sub = item.username.ifEmpty { item.url.ifEmpty { if (item.kind == "note") "Secure note" else "" } }
                            if (sub.isNotEmpty()) Text(sub, color = Muted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                        if (item.favorite) Icon(Icons.Default.Star, "Favorite", tint = Warning, modifier = Modifier.size(16.dp))
                    }
                }
                if (filtered.isEmpty()) item {
                    Text("Nothing matches.", color = Muted, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(24.dp))
                }
                item { Spacer(Modifier.height(12.dp)) }
            }
            Text(
                "Synced ${ago(app.lastSyncAt)} · edit passwords in Remvyn on your computer",
                color = Muted, fontSize = 11.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(10.dp),
            )
        }
    }
}

/** Secrets hide again after this long, like the desktop. */
private const val REVEAL_MS = 30_000L

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PasswordItemScreen(app: AppState, itemId: String) {
    val ctx = LocalContext.current
    val item = app.vault?.items?.firstOrNull { it.id == itemId }
    BackHandler { app.screen = Screen.Passwords }
    if (item == null) {
        // Deleted on another device while open.
        LaunchedEffect(Unit) { app.screen = Screen.Passwords }
        return
    }

    Scaffold(
        containerColor = Background,
        topBar = {
            TopAppBar(
                title = { Text(item.name.ifEmpty { "Untitled" }, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { IconButton(onClick = { app.screen = Screen.Passwords }) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back") } },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Background),
            )
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
            if (item.username.isNotEmpty()) Field(app, "Username", item.username, secret = false)
            if (item.password.isNotEmpty()) Field(app, if (item.kind == "token") "Token" else "Password", item.password, secret = true)
            if (item.totp.isNotEmpty()) TotpField(app, item)
            if (item.url.isNotEmpty()) {
                Field(app, "Website", item.url, secret = false, mono = false) {
                    IconButton(onClick = { openUrl(ctx, if ("://" in item.url) item.url else "https://${item.url}") }) {
                        Icon(Icons.AutoMirrored.Filled.OpenInNew, "Open", tint = Muted, modifier = Modifier.size(18.dp))
                    }
                }
            }
            item.fields.forEach { f -> Field(app, f.name.ifEmpty { "Field" }, f.value, secret = f.hidden) }
            if (item.notes.isNotEmpty()) Field(app, "Notes", item.notes, secret = item.kind == "note", mono = false, multiline = true)
            Text(
                "Updated ${ago(item.updatedAt)}. Copied secrets are cleared from the clipboard after 30 seconds and when Remvyn locks.",
                color = Muted, fontSize = 12.sp, modifier = Modifier.padding(vertical = 16.dp),
            )
        }
    }
}

@Composable
private fun Field(
    app: AppState, label: String, value: String, secret: Boolean, mono: Boolean = true, multiline: Boolean = false,
    extra: @Composable () -> Unit = {},
) {
    var shownAt by remember(value) { mutableLongStateOf(0L) }
    val shown = !secret || shownAt > 0
    LaunchedEffect(shownAt) {
        if (shownAt > 0) {
            delay(REVEAL_MS)
            shownAt = 0
        }
    }
    Column(Modifier.fillMaxWidth().padding(top = 10.dp).background(Surface, RoundedCornerShape(12.dp)).padding(start = 14.dp, top = 10.dp, bottom = 6.dp, end = 4.dp)) {
        Text(label.uppercase(), color = Muted, fontSize = 11.sp, fontWeight = FontWeight.Medium, letterSpacing = 1.sp)
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                if (shown) value else "•".repeat(12),
                fontFamily = if (mono || !shown) FontFamily.Monospace else FontFamily.Default, fontSize = 15.sp,
                maxLines = if (multiline && shown) Int.MAX_VALUE else 1, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f).padding(vertical = 4.dp),
            )
            extra()
            if (secret) {
                IconButton(onClick = { shownAt = if (shown) 0 else System.currentTimeMillis() }) {
                    Icon(if (shown) Icons.Default.VisibilityOff else Icons.Default.Visibility, if (shown) "Hide" else "Reveal",
                        tint = Muted, modifier = Modifier.size(18.dp))
                }
            }
            IconButton(onClick = { app.copySecret(label, value) }) {
                Icon(Icons.Default.ContentCopy, "Copy", tint = Muted, modifier = Modifier.size(18.dp))
            }
        }
    }
}

@Composable
private fun TotpField(app: AppState, item: VaultItem) {
    val totp = remember(item.totp) { runCatching { Totp.parse(item.totp) } }
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            now = System.currentTimeMillis()
            delay(1000 - now % 1000)
        }
    }
    Column(Modifier.fillMaxWidth().padding(top = 10.dp).background(Surface, RoundedCornerShape(12.dp)).padding(start = 14.dp, top = 10.dp, bottom = 10.dp, end = 4.dp)) {
        Text("TWO-FACTOR CODE", color = Muted, fontSize = 11.sp, fontWeight = FontWeight.Medium, letterSpacing = 1.sp)
        totp.onFailure { Text(it.message ?: "Invalid two-factor secret", color = Danger, fontSize = 13.sp, modifier = Modifier.padding(vertical = 6.dp)) }
        totp.onSuccess { t ->
            val (code, left) = t.at(now)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    code.chunked(if (code.length == 6) 3 else 4).joinToString(" "),
                    fontFamily = FontFamily.Monospace, fontSize = 26.sp, fontWeight = FontWeight.SemiBold,
                    color = if (left <= 5) Warning else Primary, modifier = Modifier.weight(1f).padding(vertical = 4.dp),
                )
                Text("${left}s", color = Muted, fontSize = 13.sp, fontFamily = FontFamily.Monospace)
                IconButton(onClick = { app.copySecret("Two-factor code", code) }) {
                    Icon(Icons.Default.ContentCopy, "Copy code", tint = Muted, modifier = Modifier.size(18.dp))
                }
            }
            LinearProgressIndicator(
                progress = { left / t.period.toFloat() }, color = if (left <= 5) Warning else Primary, trackColor = SurfaceHigh,
                modifier = Modifier.fillMaxWidth().padding(end = 10.dp, top = 4.dp),
            )
        }
    }
}
