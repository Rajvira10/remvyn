package io.github.rajvira10.flux.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.rajvira10.flux.BuildConfig
import io.github.rajvira10.flux.crypto.WrongPasswordException
import io.github.rajvira10.flux.sync.DeviceCode
import io.github.rajvira10.flux.sync.GitHub
import io.github.rajvira10.flux.sync.GitHubException
import io.github.rajvira10.flux.sync.Repo
import io.github.rajvira10.flux.sync.SyncConfig

fun openUrl(ctx: Context, url: String) {
    runCatching { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
}

fun copy(ctx: Context, text: String) {
    (ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("Flux", text))
}

@Composable
private fun Centered(content: @Composable () -> Unit) {
    Column(
        Modifier.fillMaxSize().background(Background).safeDrawingPadding().imePadding().verticalScroll(rememberScrollState()).padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) { content() }
}

private enum class Step { WELCOME, CODE, REPO, PASSWORD }

@Composable
fun SetupScreen(app: AppState) {
    val ctx = LocalContext.current
    val saved = remember { app.store.config() }
    var step by remember { mutableStateOf(Step.WELCOME) }
    var clientId by remember { mutableStateOf(BuildConfig.GITHUB_CLIENT_ID.ifEmpty { saved.clientId }) }
    var slug by remember { mutableStateOf(BuildConfig.GITHUB_APP_SLUG.ifEmpty { saved.appSlug }) }
    var web by remember { mutableStateOf(saved.web) }
    var api by remember { mutableStateOf(saved.api) }
    var advanced by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var code by remember { mutableStateOf<DeviceCode?>(null) }
    var gh by remember { mutableStateOf<GitHub?>(null) }
    var login by remember { mutableStateOf("") }
    var repos by remember { mutableStateOf<List<Repo>?>(null) }
    var installed by remember { mutableStateOf(true) }
    var picked by remember { mutableStateOf<Repo?>(null) }
    var password by remember { mutableStateOf("") }
    // Plain atomic, not Compose state: it's read from the sign-in thread.
    val attempt = remember { java.util.concurrent.atomic.AtomicInteger() }

    fun loadRepos() {
        val g = gh ?: return
        busy = true
        Thread {
            val r = runCatching { g.accessibleRepos() }
            onMain {
                busy = false
                r.onSuccess { list ->
                    installed = list != null
                    repos = list ?: emptyList()
                    if (picked == null) picked = list?.firstOrNull { it.private && it.fullName.contains("flux", true) }
                }
                r.onFailure { error = it.message }
            }
        }.start()
    }

    fun signIn() {
        error = null
        val g = GitHub(web.trimEnd('/'), api.trimEnd('/'), clientId.trim())
        gh = g
        code = null
        step = Step.CODE
        val my = attempt.incrementAndGet()
        Thread {
            try {
                val dc = g.startDeviceFlow()
                onMain { if (attempt.get() == my) code = dc }
                g.pollToken(dc) { attempt.get() != my }
                val who = g.user()
                onMain {
                    if (attempt.get() != my) return@onMain
                    login = who
                    step = Step.REPO
                    loadRepos()
                }
            } catch (e: Exception) {
                android.util.Log.w("Flux", "sign-in failed", e)
                onMain {
                    if (attempt.get() != my || (e as? GitHubException)?.code == "cancelled") return@onMain
                    error = e.message
                    step = Step.WELCOME
                }
            }
        }.start()
    }

    fun join() {
        val g = gh ?: return
        val repo = picked ?: return
        busy = true
        error = null
        Thread {
            try {
                app.store.saveConfig(SyncConfig(web.trimEnd('/'), api.trimEnd('/'), clientId.trim(), slug.trim(), repo.fullName,
                    repo.defaultBranch, login, g.token))
                app.store.join(g, repo, password)
                onMain { busy = false; password = ""; app.unlocked() }
            } catch (e: WrongPasswordException) {
                onMain { busy = false; error = "That isn’t the master password for this vault." }
            } catch (e: Exception) {
                onMain { busy = false; error = e.message ?: "Couldn't download the vault" }
            }
        }.start()
    }

    DisposableEffect(Unit) { onDispose { attempt.incrementAndGet() } }

    Centered {
        Logo()
        Spacer(Modifier.height(18.dp))
        when (step) {
            Step.WELCOME -> {
                Text("Flux", fontSize = 26.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Your hosts, keys and snippets from Flux on your computer, with an SSH terminal. " +
                        "Turn on sync on your computer first (Settings → Sync), then sign in here with the same GitHub account.",
                    color = Muted, textAlign = TextAlign.Center, fontSize = 14.sp,
                )
                Spacer(Modifier.height(22.dp))
                if (BuildConfig.GITHUB_CLIENT_ID.isEmpty()) {
                    OutlinedTextField(clientId, { clientId = it }, label = { Text("GitHub app client ID") }, singleLine = true,
                        modifier = Modifier.fillMaxWidth(), supportingText = { Text("The same app you set up in Flux on your computer") })
                    OutlinedTextField(slug, { slug = it }, label = { Text("GitHub app name (optional)") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                }
                TextButton(onClick = { advanced = !advanced }) { Text(if (advanced) "Hide advanced" else "Advanced", color = Muted) }
                if (advanced) {
                    OutlinedTextField(web, { web = it }, label = { Text("GitHub URL") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(api, { api = it }, label = { Text("GitHub API URL") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                }
                error?.let { Spacer(Modifier.height(8.dp)); Text(it, color = Danger, fontSize = 13.sp, textAlign = TextAlign.Center) }
                Spacer(Modifier.height(16.dp))
                Button(onClick = { signIn() }, enabled = clientId.isNotBlank(), modifier = Modifier.fillMaxWidth().height(48.dp)) {
                    Text("Sign in with GitHub")
                }
            }

            Step.CODE -> {
                Text("Approve on GitHub", fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(8.dp))
                Text("Enter this code on github.com to let Flux read your vault.", color = Muted, textAlign = TextAlign.Center)
                Spacer(Modifier.height(22.dp))
                val c = code
                if (c == null) {
                    CircularProgressIndicator(Modifier.size(28.dp))
                } else {
                    Text(
                        c.userCode, fontFamily = FontFamily.Monospace, fontSize = 30.sp, fontWeight = FontWeight.Bold, letterSpacing = 4.sp,
                        modifier = Modifier.border(1.dp, MaterialTheme.colorScheme.outline, RoundedCornerShape(12.dp))
                            .background(Surface, RoundedCornerShape(12.dp)).padding(horizontal = 22.dp, vertical = 12.dp),
                    )
                    Spacer(Modifier.height(18.dp))
                    Button(onClick = { copy(ctx, c.userCode); openUrl(ctx, c.verificationUri) }, modifier = Modifier.fillMaxWidth().height(48.dp)) {
                        Text("Copy code and open GitHub")
                    }
                    Spacer(Modifier.height(14.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.size(8.dp))
                        Text("Waiting for approval…", color = Muted, fontSize = 13.sp)
                    }
                }
                Spacer(Modifier.height(12.dp))
                TextButton(onClick = { attempt.incrementAndGet(); step = Step.WELCOME }) { Text("Cancel") }
            }

            Step.REPO -> {
                Text("Choose your vault", fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(6.dp))
                Text("Signed in as $login. Pick the private repository your computer syncs to.", color = Muted, textAlign = TextAlign.Center)
                Spacer(Modifier.height(16.dp))
                val list = repos
                if (list == null) CircularProgressIndicator(Modifier.size(28.dp))
                else {
                    list.forEach { r ->
                        Row(
                            Modifier.fillMaxWidth().padding(vertical = 3.dp)
                                .background(if (picked == r) SurfaceHigh else Surface, RoundedCornerShape(10.dp))
                                .clickable { picked = r }.padding(14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(r.fullName, fontFamily = FontFamily.Monospace, fontSize = 13.sp, modifier = Modifier.weight(1f))
                            if (!r.private) Text("public", color = Warning, fontSize = 12.sp)
                            if (picked == r) Icon(Icons.Default.Check, null, tint = Primary, modifier = Modifier.size(18.dp))
                        }
                    }
                    if (list.isEmpty()) {
                        Text(
                            if (installed) "Flux can’t see any repositories yet." else "The Flux GitHub app isn’t installed on your account yet.",
                            color = Muted, textAlign = TextAlign.Center,
                        )
                    }
                    Spacer(Modifier.height(8.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        if (slug.isNotBlank()) {
                            OutlinedButton(onClick = { openUrl(ctx, "${web.trimEnd('/')}/apps/${slug.trim().substringAfterLast("/apps/").trim('/')}/installations/new") }) {
                                Text("Grant access")
                            }
                        }
                        OutlinedButton(onClick = { loadRepos() }, enabled = !busy) {
                            Icon(Icons.Default.Refresh, null, Modifier.size(16.dp)); Spacer(Modifier.size(6.dp)); Text("Check again")
                        }
                    }
                }
                error?.let { Spacer(Modifier.height(8.dp)); Text(it, color = Danger, fontSize = 13.sp, textAlign = TextAlign.Center) }
                Spacer(Modifier.height(16.dp))
                Button(onClick = { error = null; step = Step.PASSWORD }, enabled = picked?.private == true, modifier = Modifier.fillMaxWidth().height(48.dp)) {
                    Text("Continue")
                }
            }

            Step.PASSWORD -> {
                Text("Master password", fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(6.dp))
                Text("Enter the master password you use in Flux on your computer. It never leaves this phone.", color = Muted, textAlign = TextAlign.Center)
                Spacer(Modifier.height(18.dp))
                val focus = remember { FocusRequester() }
                LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
                OutlinedTextField(
                    password, { password = it; error = null }, label = { Text("Master password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { if (password.isNotEmpty() && !busy) join() }),
                    modifier = Modifier.fillMaxWidth().focusRequester(focus),
                )
                error?.let { Spacer(Modifier.height(8.dp)); Text(it, color = Danger, fontSize = 13.sp, textAlign = TextAlign.Center) }
                Spacer(Modifier.height(16.dp))
                Button(onClick = { join() }, enabled = password.isNotEmpty() && !busy, modifier = Modifier.fillMaxWidth().height(48.dp)) {
                    if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text("Open vault")
                }
                TextButton(onClick = { step = Step.REPO }) { Text("Back") }
            }
        }
    }
}

@Composable
fun UnlockScreen(app: AppState) {
    var password by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var confirmErase by remember { mutableStateOf(false) }
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }

    fun unlock() {
        busy = true
        error = null
        Thread {
            val r = runCatching { app.store.unlock(password) }
            onMain {
                busy = false
                r.onSuccess { password = ""; app.unlocked() }
                r.onFailure { error = if (it is WrongPasswordException) "That password is not correct." else it.message }
            }
        }.start()
    }

    Centered {
        Logo()
        Spacer(Modifier.height(18.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Default.Lock, null, tint = Muted, modifier = Modifier.size(16.dp))
            Spacer(Modifier.size(6.dp))
            Text("Flux is locked", fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
        }
        Spacer(Modifier.height(6.dp))
        Text("Enter your master password.", color = Muted)
        Spacer(Modifier.height(20.dp))
        OutlinedTextField(
            password, { password = it; error = null }, label = { Text("Master password") }, singleLine = true,
            visualTransformation = PasswordVisualTransformation(), isError = error != null,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = { if (password.isNotEmpty() && !busy) unlock() }),
            modifier = Modifier.fillMaxWidth().focusRequester(focus),
        )
        error?.let { Spacer(Modifier.height(8.dp)); Text(it, color = Danger, fontSize = 13.sp) }
        Spacer(Modifier.height(16.dp))
        Button(onClick = { unlock() }, enabled = password.isNotEmpty() && !busy, modifier = Modifier.fillMaxWidth().height(48.dp)) {
            if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text("Unlock")
        }
        Spacer(Modifier.height(8.dp))
        if (!confirmErase) {
            TextButton(onClick = { confirmErase = true }) { Text("Forgot password?", color = Muted) }
        } else {
            Text(
                "The password can’t be recovered. You can sign out, which removes the vault from this phone; it stays safe in GitHub.",
                color = Muted, fontSize = 13.sp, textAlign = TextAlign.Center,
            )
            TextButton(onClick = { app.signOut() }) { Text("Sign out of this phone", color = Danger) }
        }
    }
}
