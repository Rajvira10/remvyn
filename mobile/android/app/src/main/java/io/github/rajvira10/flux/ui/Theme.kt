package io.github.rajvira10.flux.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.res.painterResource
import io.github.rajvira10.flux.R
import androidx.compose.material3.Text
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

val Primary = Color(0xFF7AA2F7)
val Background = Color(0xFF111216)
val Surface = Color(0xFF181A20)
val SurfaceHigh = Color(0xFF20232B)
val Muted = Color(0xFF8B90A0)
val Danger = Color(0xFFF7768E)
val Success = Color(0xFF9ECE6A)
val Warning = Color(0xFFE0AF68)

private val scheme = darkColorScheme(
    primary = Primary,
    onPrimary = Color(0xFF0B1020),
    background = Background,
    surface = Surface,
    surfaceVariant = SurfaceHigh,
    surfaceContainer = Surface,
    surfaceContainerHigh = SurfaceHigh,
    surfaceContainerHighest = Color(0xFF292C36),
    onSurface = Color(0xFFE3E5EC),
    onSurfaceVariant = Muted,
    outline = Color(0xFF353946),
    outlineVariant = Color(0xFF2A2D37),
    error = Danger,
)

@Composable
fun FluxTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = scheme, content = content)
}

/** The Flux bolt mark. */
@Composable
fun Logo(size: Dp = 56.dp) {
    Box(
        Modifier.size(size).background(Primary.copy(alpha = 0.15f), RoundedCornerShape(size / 4)),
        contentAlignment = Alignment.Center,
    ) {
        Icon(painterResource(R.drawable.ic_launcher_foreground), null, tint = Primary, modifier = Modifier.size(size * 1.3f))
    }
}

/** Two-letter avatar for a host. */
@Composable
fun Initials(label: String, color: Color = Primary) {
    val text = label.split(Regex("[\\s._-]+")).filter { it.isNotEmpty() }.take(2).joinToString("") { it.first().uppercase() }
        .ifEmpty { "?" }
    Box(Modifier.size(38.dp).background(color.copy(alpha = 0.16f), RoundedCornerShape(10.dp)), contentAlignment = Alignment.Center) {
        Text(text, color = color, fontWeight = FontWeight.SemiBold, fontSize = 13.sp)
    }
}

fun parseColor(hex: String): Color? = runCatching {
    if (hex.isBlank()) null else Color(android.graphics.Color.parseColor(hex))
}.getOrNull()
