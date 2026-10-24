package io.github.rajvira10.remvyn.crypto

import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import org.json.JSONObject
import java.util.Base64
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

class WrongPasswordException : Exception("That master password is not correct")

/**
 * Mirrors the desktop's backend/security and backend/vaultsync formats:
 * vault.json holds a 32-byte sync key wrapped with an Argon2id-derived key,
 * and every record is AES-256-GCM with its repository path as associated data.
 */
object VaultCrypto {
    private val b64 = Base64.getDecoder()

    fun argon2id(password: String, salt: ByteArray, time: Int, memoryKiB: Int, threads: Int): ByteArray {
        val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
            .withVersion(Argon2Parameters.ARGON2_VERSION_13)
            .withSalt(salt)
            .withIterations(time)
            .withMemoryAsKB(memoryKiB)
            .withParallelism(threads)
            .build()
        val gen = Argon2BytesGenerator()
        gen.init(params)
        val out = ByteArray(32)
        gen.generateBytes(password.toByteArray(Charsets.UTF_8), out)
        return out
    }

    /** Opens nonce(12) || ciphertext || tag. */
    fun gcmOpen(key: ByteArray, sealed: ByteArray, aad: ByteArray? = null): ByteArray {
        require(sealed.size > 12 + 16) { "ciphertext too short" }
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, sealed, 0, 12))
        if (aad != null) c.updateAAD(aad)
        return c.doFinal(sealed, 12, sealed.size - 12)
    }

    fun hmac(key: ByteArray, data: ByteArray): ByteArray {
        val m = Mac.getInstance("HmacSHA256")
        m.init(SecretKeySpec(key, "HmacSHA256"))
        return m.doFinal(data)
    }

    /** Unwraps the sync key in vault.json with the master password. */
    fun unwrapSyncKey(vaultJson: String, password: String): ByteArray {
        val doc = JSONObject(vaultJson)
        require(doc.optString("format") == "flux-vault") { "This repository doesn't contain a Remvyn vault" }
        require(doc.optInt("version", 1) <= 1) { "This vault was made by a newer Remvyn; update the app" }
        val key = doc.getJSONObject("key")
        val kdf = key.getJSONObject("kdf")
        require(kdf.getString("alg") == "argon2id") { "Unsupported key derivation" }
        val time = kdf.getInt("time")
        val mem = kdf.getInt("memoryKiB")
        val threads = kdf.getInt("threads")
        require(time in 1..20 && mem in 8 * 1024..1024 * 1024 && threads in 1..255) { "Invalid key derivation parameters" }
        val kek = argon2id(password, b64.decode(kdf.getString("salt")), time, mem, threads)
        val unwrapped = try {
            gcmOpen(kek, b64.decode(key.getString("wrapped")))
        } catch (e: AEADBadTagException) {
            throw WrongPasswordException()
        }
        if (unwrapped.size != 32) throw WrongPasswordException()
        return unwrapped
    }

    fun keyIdOf(vaultJson: String): String = JSONObject(vaultJson).optString("keyId")
}

fun ByteArray.hex(): String = joinToString("") { "%02x".format(it) }

/** A decrypted record. */
data class Payload(val kind: String, val id: String, val changedAt: Long, val data: JSONObject)

class RecordCipher(syncKey: ByteArray) {
    private val encKey = VaultCrypto.hmac(syncKey, "flux-sync-records-v1".toByteArray())
    private val nameKey = VaultCrypto.hmac(syncKey, "flux-sync-names-v1".toByteArray())
    val keyId: String = VaultCrypto.hmac(syncKey, "flux-sync-key-id-v1".toByteArray()).copyOf(12).hex()

    fun path(kind: String, id: String): String =
        "records/" + VaultCrypto.hmac(nameKey, (kind + "\u0000" + id).toByteArray()).copyOf(16).hex()

    fun open(path: String, content: String): Payload {
        val sealed = Base64.getDecoder().decode(content.trim())
        val plain = VaultCrypto.gcmOpen(encKey, sealed, path.toByteArray())
        val o = JSONObject(String(plain, Charsets.UTF_8))
        val p = Payload(o.getString("kind"), o.getString("id"), o.optLong("changedAt"), o.getJSONObject("data"))
        require(path(p.kind, p.id) == path) { "$path does not match its contents" }
        return p
    }
}
