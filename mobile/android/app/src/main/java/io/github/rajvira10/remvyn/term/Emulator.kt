package io.github.rajvira10.remvyn.term

import java.nio.ByteBuffer
import java.nio.CharBuffer
import java.nio.charset.CodingErrorAction

/**
 * A compact xterm-compatible screen model: enough of VT100/xterm for shells,
 * top, htop, less and vim (colors incl. 256/truecolor, cursor movement,
 * scroll regions, insert/delete, alternate screen, application cursor keys).
 *
 * Styles are packed into a Long: fg (26 bits) | bg (26 bits) << 26 | flags << 52.
 * Colors: 0 = default, 1..256 = palette index + 1, 0x2000000 | rgb = truecolor.
 */
class Emulator(cols: Int, rows: Int, private val reply: (String) -> Unit) {
    var cols = cols; private set
    var rows = rows; private set

    class Line(cols: Int) {
        var chars = IntArray(cols) { ' '.code }
        var styles = LongArray(cols)
        fun resize(n: Int) {
            if (n == chars.size) return
            val c = IntArray(n) { ' '.code }
            val s = LongArray(n)
            System.arraycopy(chars, 0, c, 0, minOf(n, chars.size))
            System.arraycopy(styles, 0, s, 0, minOf(n, styles.size))
            chars = c; styles = s
        }
        fun clear(from: Int, to: Int, style: Long) {
            for (i in from.coerceAtLeast(0) until to.coerceAtMost(chars.size)) {
                chars[i] = ' '.code; styles[i] = style and BG_MASK
            }
        }
    }

    private var main = ArrayList<Line>().apply { repeat(rows) { add(Line(cols)) } }
    private var alt = ArrayList<Line>().apply { repeat(rows) { add(Line(cols)) } }
    val scrollback = ArrayDeque<Line>()
    var altActive = false; private set
    val screen get() = if (altActive) alt else main

    var cursorX = 0; private set
    var cursorY = 0; private set
    var cursorVisible = true; private set
    var appCursorKeys = false; private set
    var bracketedPaste = false; private set
    private var style = 0L
    private var wrapPending = false
    private var autoWrap = true
    private var originMode = false
    private var top = 0
    private var bottom = rows - 1
    private var saved = IntArray(3)
    private var savedStyle = 0L

    // Parser
    private enum class State { GROUND, ESC, CSI, OSC, CHARSET, OSC_ESC }
    private var state = State.GROUND
    private val params = StringBuilder()
    private val decoder = Charsets.UTF_8.newDecoder()
        .onMalformedInput(CodingErrorAction.REPLACE).onUnmappableCharacter(CodingErrorAction.REPLACE)
    private val pending = ByteBuffer.allocate(64 * 1024)
    private val chars = CharBuffer.allocate(64 * 1024)

    @Synchronized
    fun append(data: ByteArray, len: Int) {
        var off = 0
        while (off < len) {
            val n = minOf(len - off, pending.remaining())
            pending.put(data, off, n)
            off += n
            pending.flip()
            decoder.decode(pending, chars, false)
            pending.compact()
            chars.flip()
            var hi: Char? = null
            while (chars.hasRemaining()) {
                val ch = chars.get()
                if (Character.isHighSurrogate(ch)) { hi = ch; continue }
                val cp = if (hi != null && Character.isLowSurrogate(ch)) Character.toCodePoint(hi, ch) else ch.code
                hi = null
                feed(cp)
            }
            chars.clear()
        }
    }

    private fun feed(c: Int) {
        when (state) {
            State.GROUND -> ground(c)
            State.ESC -> esc(c)
            State.CSI -> csi(c)
            State.OSC -> when (c) {
                0x07 -> state = State.GROUND
                0x1b -> state = State.OSC_ESC
                else -> {}
            }
            State.OSC_ESC -> state = if (c == '\\'.code) State.GROUND else State.OSC
            State.CHARSET -> state = State.GROUND
        }
    }

    private fun ground(c: Int) {
        when (c) {
            0x1b -> state = State.ESC
            0x07 -> {}
            0x08 -> { if (cursorX > 0) cursorX--; wrapPending = false }
            0x09 -> { cursorX = minOf(cols - 1, (cursorX / 8 + 1) * 8); wrapPending = false }
            0x0a, 0x0b, 0x0c -> lineFeed()
            0x0d -> { cursorX = 0; wrapPending = false }
            in 0 until 0x20, 0x7f -> {}
            else -> print(c)
        }
    }

    private fun print(c: Int) {
        if (wrapPending && autoWrap) {
            cursorX = 0
            lineFeed()
        }
        wrapPending = false
        val line = screen[cursorY]
        line.chars[cursorX] = c
        line.styles[cursorX] = style
        if (cursorX == cols - 1) wrapPending = true else cursorX++
    }

    private fun lineFeed() {
        wrapPending = false
        if (cursorY == bottom) scrollUp(1) else if (cursorY < rows - 1) cursorY++
    }

    private fun reverseIndex() {
        if (cursorY == top) scrollDown(1) else if (cursorY > 0) cursorY--
    }

    private fun blank() = Line(cols).also { it.clear(0, cols, style) }

    private fun scrollUp(n: Int) {
        repeat(n.coerceAtMost(bottom - top + 1)) {
            val removed = screen.removeAt(top)
            screen.add(bottom, blank())
            if (!altActive && top == 0) {
                scrollback.addLast(removed)
                if (scrollback.size > MAX_SCROLLBACK) scrollback.removeFirst()
            }
        }
    }

    private fun scrollDown(n: Int) {
        repeat(n.coerceAtMost(bottom - top + 1)) {
            screen.removeAt(bottom)
            screen.add(top, blank())
        }
    }

    private fun esc(c: Int) {
        state = State.GROUND
        when (c.toChar()) {
            '[' -> { params.setLength(0); state = State.CSI }
            ']' -> state = State.OSC
            '(', ')', '*', '+' -> state = State.CHARSET
            '7' -> saveCursor()
            '8' -> restoreCursor()
            'D' -> lineFeed()
            'E' -> { cursorX = 0; lineFeed() }
            'M' -> reverseIndex()
            'c' -> reset()
            else -> {}
        }
    }

    private fun saveCursor() { saved = intArrayOf(cursorX, cursorY, if (wrapPending) 1 else 0); savedStyle = style }
    private fun restoreCursor() {
        cursorX = saved[0].coerceIn(0, cols - 1); cursorY = saved[1].coerceIn(0, rows - 1)
        wrapPending = saved[2] == 1; style = savedStyle
    }

    private fun reset() {
        style = 0; top = 0; bottom = rows - 1; cursorX = 0; cursorY = 0; wrapPending = false
        autoWrap = true; originMode = false; appCursorKeys = false; cursorVisible = true
        if (altActive) { altActive = false }
        main.forEach { it.clear(0, cols, 0) }
    }

    private fun csi(c: Int) {
        if (c in 0x30..0x3f || c in 0x20..0x2f) { // params, private markers, intermediates
            if (params.length < 256) params.append(c.toChar())
            return
        }
        state = State.GROUND
        if (c !in 0x40..0x7e) return
        val raw = params.toString()
        val private = raw.startsWith("?")
        val intermediate = raw.any { it in ' '..'/' }
        val p = raw.trimStart('?', '>', '=', '<').filter { it.isDigit() || it == ';' || it == ':' }
            .split(';').map { it.substringBefore(':').toIntOrNull() ?: -1 }
        fun arg(i: Int, def: Int = 1) = p.getOrNull(i)?.takeIf { it > 0 } ?: def
        if (intermediate) return // e.g. DECSCUSR "CSI 2 q"
        when (c.toChar()) {
            'A' -> { cursorY = maxOf(if (cursorY >= top) top else 0, cursorY - arg(0)); wrapPending = false }
            'B', 'e' -> { cursorY = minOf(if (cursorY <= bottom) bottom else rows - 1, cursorY + arg(0)); wrapPending = false }
            'C', 'a' -> { cursorX = minOf(cols - 1, cursorX + arg(0)); wrapPending = false }
            'D' -> { cursorX = maxOf(0, cursorX - arg(0)); wrapPending = false }
            'E' -> { cursorX = 0; cursorY = minOf(rows - 1, cursorY + arg(0)); wrapPending = false }
            'F' -> { cursorX = 0; cursorY = maxOf(0, cursorY - arg(0)); wrapPending = false }
            'G', '`' -> { cursorX = (arg(0) - 1).coerceIn(0, cols - 1); wrapPending = false }
            'd' -> { cursorY = (arg(0) - 1 + if (originMode) top else 0).coerceIn(0, rows - 1); wrapPending = false }
            'H', 'f' -> {
                val base = if (originMode) top else 0
                cursorY = (arg(0) - 1 + base).coerceIn(0, rows - 1)
                cursorX = (arg(1) - 1).coerceIn(0, cols - 1)
                wrapPending = false
            }
            'J' -> when (arg(0, 0)) {
                0 -> { screen[cursorY].clear(cursorX, cols, style); for (y in cursorY + 1 until rows) screen[y].clear(0, cols, style) }
                1 -> { screen[cursorY].clear(0, cursorX + 1, style); for (y in 0 until cursorY) screen[y].clear(0, cols, style) }
                2 -> screen.forEach { it.clear(0, cols, style) }
                3 -> scrollback.clear()
            }
            'K' -> when (arg(0, 0)) {
                0 -> screen[cursorY].clear(cursorX, cols, style)
                1 -> screen[cursorY].clear(0, cursorX + 1, style)
                2 -> screen[cursorY].clear(0, cols, style)
            }
            'L' -> if (cursorY in top..bottom) repeat(arg(0).coerceAtMost(bottom - cursorY + 1)) {
                screen.removeAt(bottom); screen.add(cursorY, blank())
            }
            'M' -> if (cursorY in top..bottom) repeat(arg(0).coerceAtMost(bottom - cursorY + 1)) {
                screen.removeAt(cursorY); screen.add(bottom, blank())
            }
            'P' -> {
                val l = screen[cursorY]; val n = arg(0).coerceAtMost(cols - cursorX)
                System.arraycopy(l.chars, cursorX + n, l.chars, cursorX, cols - cursorX - n)
                System.arraycopy(l.styles, cursorX + n, l.styles, cursorX, cols - cursorX - n)
                l.clear(cols - n, cols, style)
            }
            '@' -> {
                val l = screen[cursorY]; val n = arg(0).coerceAtMost(cols - cursorX)
                System.arraycopy(l.chars, cursorX, l.chars, cursorX + n, cols - cursorX - n)
                System.arraycopy(l.styles, cursorX, l.styles, cursorX + n, cols - cursorX - n)
                l.clear(cursorX, cursorX + n, style)
            }
            'X' -> screen[cursorY].clear(cursorX, cursorX + arg(0), style)
            'S' -> scrollUp(arg(0))
            'T' -> if (!private) scrollDown(arg(0))
            'm' -> sgr(p)
            'r' -> if (!private) {
                val t = arg(0) - 1; val b = arg(1, rows) - 1
                if (t < b && b < rows) { top = t; bottom = b }
                cursorX = 0; cursorY = if (originMode) top else 0; wrapPending = false
            }
            's' -> saveCursor()
            'u' -> restoreCursor()
            'h', 'l' -> setModes(private, p, c == 'h'.code)
            'n' -> when (arg(0, 0)) {
                5 -> reply("\u001b[0n")
                6 -> reply("\u001b[${cursorY + 1};${cursorX + 1}R")
            }
            'c' -> if (!raw.startsWith(">")) reply("\u001b[?62;22c") else reply("\u001b[>1;10;0c")
            else -> {}
        }
    }

    private fun setModes(private: Boolean, p: List<Int>, on: Boolean) {
        if (!private) return
        for (m in p) when (m) {
            1 -> appCursorKeys = on
            6 -> { originMode = on; cursorX = 0; cursorY = if (on) top else 0 }
            7 -> autoWrap = on
            25 -> cursorVisible = on
            47, 1047, 1049 -> {
                if (on == altActive) continue
                if (m == 1049 && on) saveCursor()
                altActive = on
                if (on) alt.forEach { it.clear(0, cols, 0) }
                if (m == 1049 && !on) restoreCursor()
                wrapPending = false
            }
            2004 -> bracketedPaste = on
        }
    }

    private fun sgr(p: List<Int>) {
        var i = 0
        if (p.isEmpty() || (p.size == 1 && p[0] <= 0)) { style = 0; return }
        while (i < p.size) {
            val v = p[i].coerceAtLeast(0)
            when (v) {
                0 -> style = 0
                1 -> style = style or BOLD
                2 -> style = style or DIM
                3 -> style = style or ITALIC
                4 -> style = style or UNDERLINE
                7 -> style = style or INVERSE
                22 -> style = style and (BOLD or DIM).inv()
                23 -> style = style and ITALIC.inv()
                24 -> style = style and UNDERLINE.inv()
                27 -> style = style and INVERSE.inv()
                in 30..37 -> style = withFg(v - 30 + 1L)
                39 -> style = withFg(0)
                in 40..47 -> style = withBg(v - 40 + 1L)
                49 -> style = withBg(0)
                in 90..97 -> style = withFg(v - 90 + 8 + 1L)
                in 100..107 -> style = withBg(v - 100 + 8 + 1L)
                38, 48 -> {
                    val mode = p.getOrNull(i + 1)
                    var color = -1L
                    if (mode == 5) { color = (p.getOrNull(i + 2) ?: 0).coerceIn(0, 255) + 1L; i += 2 }
                    else if (mode == 2) {
                        val r = (p.getOrNull(i + 2) ?: 0).coerceIn(0, 255)
                        val g = (p.getOrNull(i + 3) ?: 0).coerceIn(0, 255)
                        val b = (p.getOrNull(i + 4) ?: 0).coerceIn(0, 255)
                        color = TRUE or ((r shl 16) or (g shl 8) or b).toLong(); i += 4
                    }
                    if (color >= 0) style = if (v == 38) withFg(color) else withBg(color)
                }
            }
            i++
        }
    }

    private fun withFg(c: Long) = (style and FG_MASK.inv()) or c
    private fun withBg(c: Long) = (style and BG_MASK.inv()) or (c shl 26)

    @Synchronized
    fun resize(newCols: Int, newRows: Int) {
        if (newCols == cols && newRows == rows || newCols < 2 || newRows < 2) return
        for (buf in listOf(main, alt)) {
            buf.forEach { it.resize(newCols) }
            while (buf.size > newRows) {
                // Drop blank lines below the cursor first, then push the top into scrollback.
                if (buf === screen && cursorY < buf.size - 1 && buf.last().chars.all { it == ' '.code }) buf.removeAt(buf.size - 1)
                else {
                    val l = buf.removeAt(0)
                    if (buf === main) scrollback.addLast(l)
                    if (buf === screen) cursorY = maxOf(0, cursorY - 1)
                }
            }
            while (buf.size < newRows) buf.add(Line(newCols))
        }
        scrollback.forEach { it.resize(newCols) }
        cols = newCols; rows = newRows
        top = 0; bottom = rows - 1
        cursorX = cursorX.coerceIn(0, cols - 1); cursorY = cursorY.coerceIn(0, rows - 1)
        wrapPending = false
    }

    /** Runs f with the lock held, so a render sees a consistent screen. */
    @Synchronized
    fun <T> read(f: Emulator.() -> T): T = f()

    companion object {
        const val MAX_SCROLLBACK = 3000
        const val FG_MASK = (1L shl 26) - 1
        const val BG_MASK = ((1L shl 26) - 1) shl 26
        const val TRUE = 0x2000000L
        const val BOLD = 1L shl 52
        const val DIM = 1L shl 53
        const val ITALIC = 1L shl 54
        const val UNDERLINE = 1L shl 55
        const val INVERSE = 1L shl 56

        fun fg(style: Long) = style and FG_MASK
        fun bg(style: Long) = (style shr 26) and FG_MASK
    }
}
