package io.github.rajvira10.remvyn.crypto

import java.net.URLDecoder
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Time-based one-time codes (RFC 6238), mirroring the desktop's backend/totp:
 * a base32 secret or an otpauth://totp link, SHA1/SHA256/SHA512, 6–10 digits.
 */
class Totp(val secret: ByteArray, val algorithm: String = "SHA1", val digits: Int = 6, val period: Int = 30) {

    /** The code for [atMillis] and how many seconds it stays valid. */
    fun at(atMillis: Long): Pair<String, Int> {
        val seconds = atMillis / 1000
        val counter = seconds / period
        val msg = ByteArray(8) { i -> (counter ushr (56 - 8 * i)).toByte() }
        val mac = Mac.getInstance("Hmac$algorithm")
        mac.init(SecretKeySpec(secret, "Hmac$algorithm"))
        val sum = mac.doFinal(msg)
        val off = sum.last().toInt() and 0x0f
        val bin = ((sum[off].toInt() and 0x7f) shl 24) or ((sum[off + 1].toInt() and 0xff) shl 16) or
            ((sum[off + 2].toInt() and 0xff) shl 8) or (sum[off + 3].toInt() and 0xff)
        var mod = 1L
        repeat(digits) { mod *= 10 }
        val code = (bin.toLong() % mod).toString().padStart(digits, '0')
        return code to (period - (seconds % period)).toInt()
    }

    companion object {
        /** Parses a base32 secret or an otpauth:// link; throws with a readable message. */
        fun parse(input: String): Totp {
            val s = input.trim()
            var secret = s
            var alg = "SHA1"
            var digits = 6
            var period = 30
            if (s.startsWith("otpauth://", ignoreCase = true)) {
                // Split by hand: labels often contain raw spaces, which java.net.URI rejects.
                val rest = s.substring("otpauth://".length)
                require(rest.substringBefore('/').substringBefore('?').equals("totp", ignoreCase = true)) { "Only time-based (TOTP) codes are supported" }
                val q = rest.substringAfter('?', "").substringBefore('#').split('&').filter { it.isNotEmpty() }.associate {
                    val (k, v) = (it.split('=', limit = 2) + "").take(2)
                    URLDecoder.decode(k, "UTF-8").lowercase() to URLDecoder.decode(v, "UTF-8")
                }
                secret = q["secret"] ?: ""
                q["algorithm"]?.takeIf { it.isNotEmpty() }?.let { alg = it.uppercase() }
                q["digits"]?.takeIf { it.isNotEmpty() }?.let {
                    digits = it.toIntOrNull() ?: throw IllegalArgumentException("The two-factor link has invalid digits")
                }
                q["period"]?.takeIf { it.isNotEmpty() }?.let {
                    period = it.toIntOrNull() ?: throw IllegalArgumentException("The two-factor link has an invalid period")
                }
            }
            require(digits in 6..10) { "Unsupported number of digits: $digits" }
            require(period in 1..3600) { "Unsupported period: $period seconds" }
            require(alg in setOf("SHA1", "SHA256", "SHA512")) { "Unsupported algorithm \"$alg\"" }
            return Totp(base32(secret), alg, digits, period)
        }

        fun base32(input: String): ByteArray {
            val s = input.uppercase().filterNot { it == ' ' || it == '-' || it == '\t' }.trimEnd('=')
            require(s.isNotEmpty()) { "The two-factor secret is empty" }
            val out = java.io.ByteArrayOutputStream()
            var buf = 0
            var bits = 0
            for (ch in s) {
                val v = ALPHABET.indexOf(ch)
                require(v >= 0) { "The two-factor secret isn't valid base32 (letters A–Z and digits 2–7)" }
                buf = (buf shl 5) or v
                bits += 5
                if (bits >= 8) {
                    bits -= 8
                    out.write((buf shr bits) and 0xff)
                }
            }
            return out.toByteArray().also { require(it.isNotEmpty()) { "The two-factor secret isn't valid base32 (letters A–Z and digits 2–7)" } }
        }

        private const val ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
    }
}
