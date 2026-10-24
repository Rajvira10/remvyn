package io.github.rajvira10.flux.term

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Typeface
import android.text.InputType
import android.view.GestureDetector
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.ScaleGestureDetector
import android.view.View
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager
import kotlin.math.abs
import kotlin.math.floor

/** Colors for a terminal theme: background, foreground, cursor and the 16 ANSI colors. */
class TermColors(val background: Int, val foreground: Int, val cursor: Int, val ansi: IntArray) {
    private val palette = IntArray(256).also { p ->
        for (i in 0 until 16) p[i] = ansi[i]
        val steps = intArrayOf(0, 95, 135, 175, 215, 255)
        for (i in 0 until 216) p[16 + i] = rgb(steps[i / 36], steps[(i / 6) % 6], steps[i % 6])
        for (i in 0 until 24) { val v = 8 + i * 10; p[232 + i] = rgb(v, v, v) }
    }

    fun color(c: Long, def: Int): Int = when {
        c == 0L -> def
        c and Emulator.TRUE != 0L -> (0xff000000L or (c and 0xffffffL)).toInt()
        else -> palette[((c - 1).toInt()).coerceIn(0, 255)]
    }

    companion object {
        fun rgb(r: Int, g: Int, b: Int) = (0xff shl 24) or (r shl 16) or (g shl 8) or b
        fun hex(s: String) = (0xff000000L or s.removePrefix("#").toLong(16)).toInt()

        /** Flux Dark, matching the desktop default. */
        val FLUX = TermColors(hex("15161a"), hex("d8dbe2"), hex("7aa2f7"), intArrayOf(
            hex("1d1f25"), hex("f7768e"), hex("9ece6a"), hex("e0af68"), hex("7aa2f7"), hex("bb9af7"), hex("7dcfff"), hex("c0caf5"),
            hex("545c7e"), hex("ff899d"), hex("b9f27c"), hex("ffc777"), hex("8db0ff"), hex("c7a9ff"), hex("a4daff"), hex("e5e9f0"),
        ))
    }
}

/**
 * Draws an [Emulator] and turns touch and keyboard input into bytes for the
 * remote shell. Pinch to change the text size; drag to scroll back.
 */
class TerminalView(context: Context) : View(context) {
    var emulator: Emulator? = null
        set(value) { field = value; scrollOffset = 0; requestLayout(); invalidate() }
    var colors = TermColors.FLUX
    var onInput: (ByteArray) -> Unit = {}
    var onResize: (cols: Int, rows: Int) -> Unit = { _, _ -> }
    var onFontSize: (Float) -> Unit = {}
    /** Sticky modifiers from the extra-keys row; cleared after one key. */
    var ctrlArmed = false
    var altArmed = false
    var onModifiersUsed: () -> Unit = {}

    private val density = resources.displayMetrics.scaledDensity
    var fontSizeSp = 13f
        set(v) { field = v.coerceIn(7f, 28f); updateMetrics(); requestLayout(); post { fitToSize() } }

    private val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { typeface = Typeface.MONOSPACE }
    private val bgPaint = Paint()
    private var cellW = 0f
    private var cellH = 0f
    private var baseline = 0f
    private var scrollOffset = 0 // lines scrolled back
    private var scrollRemainder = 0f

    init {
        isFocusable = true
        isFocusableInTouchMode = true
        updateMetrics()
    }

    private fun updateMetrics() {
        paint.textSize = fontSizeSp * density
        cellW = paint.measureText("M")
        val fm = paint.fontMetrics
        cellH = (fm.descent - fm.ascent) * 1.1f
        baseline = -fm.ascent + (cellH - (fm.descent - fm.ascent)) / 2
    }

    override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) {
        super.onSizeChanged(w, h, oldw, oldh)
        fitToSize()
    }

    private fun fitToSize() {
        if (width == 0 || height == 0) return
        val cols = floor((width - paddingLeft - paddingRight) / cellW).toInt().coerceAtLeast(10)
        val rows = floor((height - paddingTop - paddingBottom) / cellH).toInt().coerceAtLeast(4)
        val e = emulator ?: return
        if (cols != e.cols || rows != e.rows) {
            e.resize(cols, rows)
            onResize(cols, rows)
        }
        invalidate()
    }

    fun gridSize(): Pair<Int, Int> {
        if (width == 0) return 80 to 24
        return floor((width - paddingLeft - paddingRight) / cellW).toInt().coerceAtLeast(10) to
            floor((height - paddingTop - paddingBottom) / cellH).toInt().coerceAtLeast(4)
    }

    /** New output arrived: jump back to the bottom and redraw. */
    fun onScreenChanged() {
        post {
            scrollOffset = 0
            invalidate()
        }
    }

    override fun onDraw(canvas: Canvas) {
        // Stay inside our bounds: drawColor would paint over neighbouring Compose UI.
        canvas.clipRect(0, 0, width, height)
        bgPaint.color = colors.background
        canvas.drawRect(0f, 0f, width.toFloat(), height.toFloat(), bgPaint)
        val e = emulator ?: return
        e.read {
            val sb = scrollback.size
            val offset = scrollOffset.coerceAtMost(sb)
            for (row in 0 until rows) {
                val idx = row - offset
                val line = if (idx < 0) scrollback.elementAtOrNull(sb + idx) else screen.getOrNull(idx)
                line ?: continue
                drawLine(canvas, line, row, cols)
            }
            if (offset == 0 && cursorVisible && hasFocus()) {
                val x = paddingLeft + cursorX * cellW
                val y = paddingTop + cursorY * cellH
                bgPaint.color = colors.cursor
                bgPaint.alpha = 200
                canvas.drawRect(x, y, x + cellW, y + cellH, bgPaint)
                bgPaint.alpha = 255
                val ch = screen[cursorY].chars[cursorX]
                if (ch != ' '.code) {
                    paint.color = colors.background
                    canvas.drawText(String(Character.toChars(ch)), x, y + baseline, paint)
                }
            } else if (offset == 0 && cursorVisible) {
                val x = paddingLeft + cursorX * cellW
                val y = paddingTop + cursorY * cellH
                bgPaint.color = colors.cursor
                bgPaint.style = Paint.Style.STROKE
                bgPaint.strokeWidth = density
                canvas.drawRect(x, y, x + cellW, y + cellH, bgPaint)
                bgPaint.style = Paint.Style.FILL
            }
        }
    }

    private val run = StringBuilder()

    private fun drawLine(canvas: Canvas, line: Emulator.Line, row: Int, cols: Int) {
        val y = paddingTop + row * cellH
        var x = 0
        val n = minOf(cols, line.chars.size)
        while (x < n) {
            val style = line.styles[x]
            var end = x + 1
            while (end < n && line.styles[end] == style) end++
            var fg = colors.color(Emulator.fg(style), colors.foreground)
            var bg = colors.color(Emulator.bg(style), colors.background)
            if (style and Emulator.BOLD != 0L && Emulator.fg(style) in 1L..8L) fg = colors.color(Emulator.fg(style) + 8, fg)
            if (style and Emulator.INVERSE != 0L) { val t = fg; fg = bg; bg = t }
            if (bg != colors.background) {
                bgPaint.color = bg
                canvas.drawRect(paddingLeft + x * cellW, y, paddingLeft + end * cellW, y + cellH, bgPaint)
            }
            paint.color = fg
            if (style and Emulator.DIM != 0L) paint.alpha = 150
            paint.isFakeBoldText = style and Emulator.BOLD != 0L
            paint.textSkewX = if (style and Emulator.ITALIC != 0L) -0.2f else 0f
            paint.isUnderlineText = style and Emulator.UNDERLINE != 0L
            // Draw cell by cell so wide or missing glyphs can't shift the grid.
            for (i in x until end) {
                val c = line.chars[i]
                if (c == ' '.code) continue
                run.setLength(0)
                run.appendCodePoint(c)
                canvas.drawText(run, 0, run.length, paddingLeft + i * cellW, y + baseline, paint)
            }
            paint.alpha = 255
            x = end
        }
    }

    // ------------------------------------------------------------ touch

    private val gestures = GestureDetector(context, object : GestureDetector.SimpleOnGestureListener() {
        override fun onSingleTapUp(e: MotionEvent): Boolean {
            showKeyboard()
            return true
        }

        override fun onScroll(e1: MotionEvent?, e2: MotionEvent, dx: Float, dy: Float): Boolean {
            if (abs(dy) < abs(dx)) return false
            val em = emulator ?: return false
            if (em.read { altActive }) {
                // Full-screen apps (less, vim): send arrow keys instead of scrolling back.
                scrollRemainder += dy
                while (abs(scrollRemainder) >= cellH) {
                    val up = scrollRemainder < 0
                    sendKey(if (up) "A" else "B")
                    scrollRemainder += if (up) cellH else -cellH
                }
                return true
            }
            scrollRemainder -= dy
            val lines = (scrollRemainder / cellH).toInt()
            if (lines != 0) {
                scrollRemainder -= lines * cellH
                scrollOffset = (scrollOffset + lines).coerceIn(0, em.read { scrollback.size })
                invalidate()
            }
            return true
        }
    })

    private val scale = ScaleGestureDetector(context, object : ScaleGestureDetector.SimpleOnScaleGestureListener() {
        override fun onScale(d: ScaleGestureDetector): Boolean {
            fontSizeSp *= d.scaleFactor
            onFontSize(fontSizeSp)
            return true
        }
    })

    override fun onTouchEvent(event: MotionEvent): Boolean {
        scale.onTouchEvent(event)
        if (!scale.isInProgress) gestures.onTouchEvent(event)
        return true
    }

    fun showKeyboard() {
        requestFocus()
        (context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager).showSoftInput(this, InputMethodManager.SHOW_IMPLICIT)
    }

    // ------------------------------------------------------------ keyboard

    override fun onCheckIsTextEditor() = true

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        // Raw input: no suggestions or autocorrect, Enter sends a key event.
        outAttrs.inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS or InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD
        outAttrs.imeOptions = EditorInfo.IME_FLAG_NO_EXTRACT_UI or EditorInfo.IME_FLAG_NO_FULLSCREEN or EditorInfo.IME_ACTION_NONE
        return object : BaseInputConnection(this, false) {
            override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
                sendText(text.toString())
                return true
            }

            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
                repeat(beforeLength.coerceAtLeast(1)) { send("\u007f") }
                return true
            }

            override fun sendKeyEvent(event: KeyEvent): Boolean {
                if (event.action == KeyEvent.ACTION_DOWN) onKeyDown(event.keyCode, event)
                return true
            }
        }
    }

    private fun send(s: String) {
        scrollOffset = 0
        onInput(s.toByteArray(Charsets.UTF_8))
    }

    fun sendText(text: String) {
        if (text.isEmpty()) return
        var s = text.replace("\n", "\r")
        if (ctrlArmed && s.length == 1) s = ctrl(s[0])?.toString() ?: s
        if (altArmed) s = "\u001b" + s
        if (ctrlArmed || altArmed) { ctrlArmed = false; altArmed = false; onModifiersUsed() }
        send(s)
    }

    /** Sends a cursor key honouring application cursor mode (A/B/C/D). */
    fun sendKey(final: String) {
        val app = emulator?.read { appCursorKeys } == true
        send(if (app) "\u001bO$final" else "\u001b[$final")
    }

    fun paste(text: String) {
        val bracketed = emulator?.read { bracketedPaste } == true
        val body = text.replace("\r\n", "\r").replace("\n", "\r")
        send(if (bracketed) "\u001b[200~$body\u001b[201~" else body)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        val seq: String? = when (keyCode) {
            KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> "\r"
            KeyEvent.KEYCODE_DEL -> "\u007f"
            KeyEvent.KEYCODE_FORWARD_DEL -> "\u001b[3~"
            KeyEvent.KEYCODE_TAB -> "\t"
            KeyEvent.KEYCODE_ESCAPE -> "\u001b"
            KeyEvent.KEYCODE_DPAD_UP -> { sendKey("A"); return true }
            KeyEvent.KEYCODE_DPAD_DOWN -> { sendKey("B"); return true }
            KeyEvent.KEYCODE_DPAD_RIGHT -> { sendKey("C"); return true }
            KeyEvent.KEYCODE_DPAD_LEFT -> { sendKey("D"); return true }
            KeyEvent.KEYCODE_MOVE_HOME -> "\u001b[H"
            KeyEvent.KEYCODE_MOVE_END -> "\u001b[F"
            KeyEvent.KEYCODE_PAGE_UP -> "\u001b[5~"
            KeyEvent.KEYCODE_PAGE_DOWN -> "\u001b[6~"
            else -> null
        }
        if (seq != null) {
            send(if (event.isAltPressed) "\u001b$seq" else seq)
            return true
        }
        val uc = event.getUnicodeChar(event.metaState and (KeyEvent.META_CTRL_MASK or KeyEvent.META_ALT_MASK).inv())
        if (uc != 0) {
            val ch = uc.toChar()
            if (event.isCtrlPressed) {
                ctrl(ch)?.let { send((if (event.isAltPressed) "\u001b" else "") + it); return true }
            }
            sendText((if (event.isAltPressed) "\u001b" else "") + String(Character.toChars(uc)))
            return true
        }
        return super.onKeyDown(keyCode, event)
    }

    companion object {
        fun ctrl(c: Char): Char? = when (c) {
            in 'a'..'z' -> (c - 'a' + 1).toChar()
            in 'A'..'Z' -> (c - 'A' + 1).toChar()
            '@', ' ', '2' -> 0.toChar()
            '[', '3' -> 27.toChar()
            '\\', '4' -> 28.toChar()
            ']', '5' -> 29.toChar()
            '^', '6' -> 30.toChar()
            '_', '-', '7' -> 31.toChar()
            '?', '8' -> 127.toChar()
            else -> null
        }
    }
}
