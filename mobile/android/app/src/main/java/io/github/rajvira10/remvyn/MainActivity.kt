package io.github.rajvira10.remvyn

import android.os.Bundle
import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.SystemBarStyle
import androidx.activity.enableEdgeToEdge
import android.graphics.Color
import androidx.compose.material3.Surface
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import io.github.rajvira10.remvyn.ui.AppState
import io.github.rajvira10.remvyn.ui.Background
import io.github.rajvira10.remvyn.ui.RemvynTheme
import io.github.rajvira10.remvyn.ui.HostsScreen
import io.github.rajvira10.remvyn.ui.Screen
import io.github.rajvira10.remvyn.ui.SettingsScreen
import io.github.rajvira10.remvyn.ui.SetupScreen
import io.github.rajvira10.remvyn.ui.TerminalScreen
import io.github.rajvira10.remvyn.ui.UnlockScreen
import android.view.WindowManager

class MainActivity : ComponentActivity() {
    private lateinit var app: AppState
    private var stoppedAt = 0L

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        // Keep the vault and terminals out of screenshots and the app switcher.
        if (!BuildConfig.DEBUG) window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        app = AppState(this)
        setContent {
            RemvynTheme {
                Surface(Modifier.fillMaxSize(), color = Background) {
                    when (val s = app.screen) {
                        Screen.Setup -> SetupScreen(app)
                        Screen.Unlock -> UnlockScreen(app)
                        Screen.Hosts -> HostsScreen(app)
                        is Screen.Terminal -> TerminalScreen(app, s.hostId)
                        Screen.Settings -> SettingsScreen(app)
                    }
                }
            }
        }
    }

    override fun onStop() {
        super.onStop()
        stoppedAt = SystemClock.elapsedRealtime()
    }

    override fun onStart() {
        super.onStart()
        if (stoppedAt > 0 && SystemClock.elapsedRealtime() - stoppedAt > LOCK_AFTER_MS && app.store.unlocked) app.lock()
        else if (app.store.unlocked && app.vault != null) app.sync()
        stoppedAt = 0
    }

    companion object {
        const val LOCK_AFTER_MS = 5 * 60 * 1000L
    }
}
