package io.github.rajvira10.remvyn.sync

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import io.github.rajvira10.remvyn.crypto.RecordCipher
import io.github.rajvira10.remvyn.crypto.VaultCrypto
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.security.KeyStore
import java.util.Base64
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

data class Host(
    val id: String, val label: String, val groupId: String, val hostname: String, val port: Int,
    val username: String, val authMethod: String, val keyId: String, val color: String,
    val startupCommand: String, val password: String,
)

data class SshKey(val id: String, val name: String, val type: String, val privateKey: String, val passphrase: String)
data class Group(val id: String, val name: String)
data class Snippet(val id: String, val name: String, val command: String, val description: String)
data class HostKey(val type: String, val key: ByteArray)

data class ItemField(val name: String, val value: String, val hidden: Boolean)

/** A password vault entry: a login, an API token or a secure note. */
data class VaultItem(
    val id: String, val kind: String, val name: String, val username: String, val url: String, val favorite: Boolean,
    val password: String, val totp: String, val notes: String, val fields: List<ItemField>, val updatedAt: Long,
) {
    companion object {
        /** Reads a "vaultitem" record as written by the desktop (backend/storage/items.go syncItem). */
        fun fromRecord(id: String, d: JSONObject): VaultItem {
            val sec = d.optJSONObject("secret") ?: JSONObject()
            val arr = sec.optJSONArray("fields") ?: JSONArray()
            val fields = (0 until arr.length()).map {
                val f = arr.getJSONObject(it)
                ItemField(f.optString("name"), f.optString("value"), f.optBoolean("hidden"))
            }
            return VaultItem(id, d.optString("kind", "login"), d.optString("name"), d.optString("username"), d.optString("url"),
                d.optBoolean("favorite"), sec.optString("password"), sec.optString("totp"), sec.optString("notes"), fields,
                d.optLong("updatedAt"))
        }
    }
}

class Vault(
    val hosts: List<Host>,
    val keys: Map<String, SshKey>,
    val groups: Map<String, Group>,
    val snippets: List<Snippet>,
    /** "host:port" -> trusted keys, from the desktop. */
    val knownHosts: Map<String, List<HostKey>>,
    /** Password vault items, favorites first. */
    val items: List<VaultItem> = emptyList(),
)

data class SyncConfig(
    val web: String = "https://github.com",
    val api: String = "https://api.github.com",
    val clientId: String = "",
    val appSlug: String = "",
    val repo: String = "",
    val branch: String = "main",
    val login: String = "",
    val token: String = "",
    val lastSyncAt: Long = 0,
)

/** One vault on this phone, synced from its own repository. */
data class VaultProfile(val id: String, val name: String, val repo: String, val branch: String = "main", val lastSyncAt: Long = 0)

/**
 * Keeps the repository's encrypted files on the device. Nothing is stored
 * decrypted: records stay encrypted with the sync key, which only exists
 * wrapped by the master password, so opening the app needs the password.
 * The GitHub token is sealed with a non-exportable Android Keystore key.
 *
 * Like the desktop, the phone can keep several vaults under one GitHub
 * sign-in, each from its own repository. The first vault keeps its cache in
 * the vault folder itself (so 2.7.0 installs carry on unchanged); the others
 * live under vaults/<id>. vaults.json lists them and says which one is open.
 */
class VaultStore(private val ctx: Context) {
    private val dir = File(ctx.filesDir, "vault").apply { mkdirs() }
    private val configFile = File(dir, "config.json")
    private val vaultsFile = File(dir, "vaults.json")
    private val trustedFile = File(dir, "trusted-hosts.json")
    private val cacheFile get() = File(vaultDir(activeId()), "cache.json")

    private var cipher: RecordCipher? = null
    var vault: Vault? = null
        private set

    val unlocked get() = cipher != null

    // ------------------------------------------------------------ config

    /** The GitHub sign-in plus the open vault's repository. */
    fun config(): SyncConfig {
        if (!configFile.exists()) return SyncConfig()
        val o = JSONObject(configFile.readText())
        val token = o.optString("token").takeIf { it.isNotEmpty() }?.let { runCatching { unsealToken(it) }.getOrDefault("") } ?: ""
        val v = profiles().firstOrNull { it.id == activeId() }
        return SyncConfig(o.optString("web", "https://github.com"), o.optString("api", "https://api.github.com"),
            o.optString("clientId"), o.optString("appSlug"), v?.repo ?: "", v?.branch ?: "main",
            o.optString("login"), token, v?.lastSyncAt ?: 0)
    }

    /** Saves the GitHub sign-in, and the repository for the open vault. */
    fun saveConfig(c: SyncConfig) {
        val o = JSONObject()
            .put("web", c.web).put("api", c.api).put("clientId", c.clientId).put("appSlug", c.appSlug)
            .put("login", c.login).put("token", if (c.token.isEmpty()) "" else sealToken(c.token))
        writeAtomic(configFile, o.toString())
        val (active, list) = loadVaults()
        val i = list.indexOfFirst { it.id == active }
        if (i < 0) list += VaultProfile(active, DEFAULT_NAME, c.repo, c.branch, c.lastSyncAt)
        else list[i] = list[i].copy(repo = c.repo, branch = c.branch, lastSyncAt = c.lastSyncAt)
        saveVaults(active, list)
    }

    /** A vault has been downloaded to this device. */
    fun hasVault(): Boolean = cacheFile.exists() && config().repo.isNotEmpty()

    /** A GitHub sign-in is saved, so another vault can be added without signing in again. */
    fun signedIn(): Boolean = config().token.isNotEmpty()

    // ------------------------------------------------------------ vaults

    fun profiles(): List<VaultProfile> = loadVaults().second

    fun activeId(): String = loadVaults().first

    fun active(): VaultProfile? = profiles().firstOrNull { it.id == activeId() }

    private fun vaultDir(id: String) = if (id == DEFAULT_ID) dir else File(dir, "vaults/$id")

    private fun loadVaults(): Pair<String, MutableList<VaultProfile>> {
        if (!vaultsFile.exists()) {
            // 2.7.0 kept one vault's repository in config.json.
            if (!configFile.exists()) return DEFAULT_ID to mutableListOf()
            val o = JSONObject(configFile.readText())
            val repo = o.optString("repo")
            if (repo.isEmpty()) return DEFAULT_ID to mutableListOf()
            return DEFAULT_ID to mutableListOf(VaultProfile(DEFAULT_ID, DEFAULT_NAME, repo, o.optString("branch", "main"), o.optLong("lastSyncAt")))
        }
        val o = JSONObject(vaultsFile.readText())
        val arr = o.optJSONArray("vaults") ?: JSONArray()
        val list = (0 until arr.length()).map {
            val v = arr.getJSONObject(it)
            VaultProfile(v.getString("id"), v.optString("name"), v.optString("repo"), v.optString("branch", "main"), v.optLong("lastSyncAt"))
        }.toMutableList()
        val active = o.optString("active").takeIf { id -> list.any { it.id == id } } ?: list.firstOrNull()?.id ?: DEFAULT_ID
        return active to list
    }

    private fun saveVaults(active: String, list: List<VaultProfile>) {
        val arr = JSONArray()
        for (v in list) {
            arr.put(JSONObject().put("id", v.id).put("name", v.name).put("repo", v.repo).put("branch", v.branch).put("lastSyncAt", v.lastSyncAt))
        }
        writeAtomic(vaultsFile, JSONObject().put("active", active).put("vaults", arr).toString())
    }

    /** Opens another vault on this phone; it starts locked. */
    fun switchTo(id: String) {
        val (_, list) = loadVaults()
        require(list.any { it.id == id }) { "No such vault" }
        lock()
        saveVaults(id, list)
    }

    fun rename(id: String, name: String) {
        val (active, list) = loadVaults()
        val i = list.indexOfFirst { it.id == id }
        if (i < 0) return
        list[i] = list[i].copy(name = name.trim().take(MAX_NAME).ifEmpty { list[i].name })
        saveVaults(active, list)
    }

    /**
     * Downloads another vault with the saved GitHub sign-in and opens it. If
     * that fails (say, a wrong password) the vault that was open stays open.
     */
    fun addVault(gh: GitHub, repo: Repo, password: String) {
        val (prevActive, list) = loadVaults()
        if (list.any { it.repo == repo.fullName }) throw GitHubException("That repository is already a vault on this phone")
        val prevCipher = cipher
        val prevVault = vault
        val id = java.util.UUID.randomUUID().toString().replace("-", "").take(12)
        val used = list.map { it.name }.toSet()
        val base = repo.fullName.substringAfter('/')
        val name = generateSequence(1) { it + 1 }.map { if (it == 1) base else "$base $it" }.first { it !in used }
        list += VaultProfile(id, name, repo.fullName, repo.defaultBranch)
        saveVaults(id, list)
        try {
            vaultDir(id).mkdirs()
            join(gh, repo, password)
        } catch (e: Exception) {
            vaultDir(id).deleteRecursively()
            saveVaults(prevActive, list.filter { it.id != id })
            cipher = prevCipher
            vault = prevVault
            throw e
        }
    }

    /** Removes a vault from this phone (it stays in GitHub). The last one signs out. */
    fun remove(id: String) {
        val (active, list) = loadVaults()
        val rest = list.filter { it.id != id }
        if (rest.isEmpty()) {
            erase()
            return
        }
        if (id == active) lock()
        if (id == DEFAULT_ID) File(dir, "cache.json").delete() else vaultDir(id).deleteRecursively()
        saveVaults(if (id == active) rest.first().id else active, rest)
    }

    // ------------------------------------------------------------ cache

    private class Cache(var commit: String, var vaultJson: String, val records: MutableMap<String, Pair<String, String>>)

    private fun loadCache(): Cache? {
        if (!cacheFile.exists()) return null
        val o = JSONObject(cacheFile.readText())
        val recs = mutableMapOf<String, Pair<String, String>>()
        val r = o.getJSONObject("records")
        for (k in r.keys()) {
            val e = r.getJSONObject(k)
            recs[k] = e.getString("sha") to e.getString("content")
        }
        return Cache(o.getString("commit"), o.getString("vaultJson"), recs)
    }

    private fun saveCache(c: Cache) {
        val r = JSONObject()
        for ((k, v) in c.records) r.put(k, JSONObject().put("sha", v.first).put("content", v.second))
        writeAtomic(cacheFile, JSONObject().put("commit", c.commit).put("vaultJson", c.vaultJson).put("records", r).toString())
    }

    // ------------------------------------------------------------ unlock

    /** Unlocks the downloaded vault with the master password. Slow (Argon2id). */
    fun unlock(password: String) {
        val cache = loadCache() ?: error("No vault on this device")
        val key = VaultCrypto.unwrapSyncKey(cache.vaultJson, password)
        cipher = RecordCipher(key)
        vault = decrypt(cache)
    }

    fun lock() {
        cipher = null
        vault = null
    }

    /**
     * First download: checks the password against the repository's vault.json,
     * then pulls everything.
     */
    fun join(gh: GitHub, repo: Repo, password: String) {
        val head = gh.ref(repo.fullName, repo.defaultBranch) ?: throw GitHubException("That repository is empty. Create the vault on your computer first.")
        val files = gh.files(repo.fullName, head)
        val vaultSha = files["vault.json"] ?: throw GitHubException("That repository doesn't contain a Remvyn vault. Set up sync on your computer first.")
        val vaultJson = gh.blob(repo.fullName, vaultSha)
        val key = VaultCrypto.unwrapSyncKey(vaultJson, password)
        cipher = RecordCipher(key)
        val cache = Cache("", vaultJson, mutableMapOf())
        pullInto(gh, repo.fullName, head, files, cache)
    }

    /** Fetches changes from the repository. Returns true if anything changed. */
    fun sync(): Boolean {
        val cfg = config()
        val c = cipher ?: error("Unlock first")
        val gh = GitHub(cfg.web, cfg.api, cfg.clientId, cfg.token)
        val cache = loadCache() ?: Cache("", "", mutableMapOf())
        val head = gh.ref(cfg.repo, cfg.branch) ?: throw GitHubException("The vault repository is empty")
        if (head == cache.commit) {
            saveConfig(cfg.copy(lastSyncAt = System.currentTimeMillis()))
            return false
        }
        val files = gh.files(cfg.repo, head)
        val vaultSha = files["vault.json"] ?: throw GitHubException("The vault file is missing from the repository", "vault_missing")
        val vaultJson = gh.blob(cfg.repo, vaultSha)
        if (VaultCrypto.keyIdOf(vaultJson) != c.keyId) {
            throw GitHubException("The vault was replaced from another device; sign out and join it again", "vault_reset")
        }
        cache.vaultJson = vaultJson // re-wrapped after a password change
        pullInto(gh, cfg.repo, head, files, cache)
        return true
    }

    private fun pullInto(gh: GitHub, repo: String, head: String, files: Map<String, String>, cache: Cache) {
        val wanted = files.filterKeys { it.startsWith("records/") }
        val missing = wanted.filter { (p, sha) -> cache.records[p]?.first != sha }
        val pool = Executors.newFixedThreadPool(6)
        try {
            val results = missing.map { (p, sha) -> pool.submit(Callable { Triple(p, sha, gh.blob(repo, sha)) }) }
            for (f in results) {
                val (p, sha, content) = f.get()
                cache.records[p] = sha to content
            }
        } catch (e: java.util.concurrent.ExecutionException) {
            throw e.cause ?: e
        } finally {
            pool.shutdownNow()
        }
        cache.records.keys.retainAll(wanted.keys)
        cache.commit = head
        vault = decrypt(cache)
        saveCache(cache)
        saveConfig(config().copy(lastSyncAt = System.currentTimeMillis()))
    }

    private fun decrypt(cache: Cache): Vault {
        val c = cipher ?: error("locked")
        val hosts = mutableListOf<Host>()
        val keys = mutableMapOf<String, SshKey>()
        val groups = mutableMapOf<String, Group>()
        val snippets = mutableListOf<Snippet>()
        val known = mutableMapOf<String, List<HostKey>>()
        val items = mutableListOf<VaultItem>()
        for ((path, rec) in cache.records) {
            val p = runCatching { c.open(path, rec.second) }.getOrNull() ?: continue
            val d = p.data
            when (p.kind) {
                "host" -> hosts += Host(p.id, d.optString("label"), d.optString("groupId"), d.optString("hostname"),
                    d.optInt("port", 22), d.optString("username"), d.optString("authMethod"), d.optString("keyId"),
                    d.optString("color"), d.optString("startupCommand"), d.optString("password"))
                "key" -> keys[p.id] = SshKey(p.id, d.optString("name"), d.optString("type"), d.optString("privateKey"), d.optString("passphrase"))
                "group" -> groups[p.id] = Group(p.id, d.optString("name"))
                "snippet" -> snippets += Snippet(p.id, d.optString("name"), d.optString("command"), d.optString("description"))
                "vaultitem" -> items += VaultItem.fromRecord(p.id, d)
                "knownhost" -> {
                    val arr: JSONArray = d.optJSONArray("keys") ?: JSONArray()
                    known[p.id] = (0 until arr.length()).map {
                        val k = arr.getJSONObject(it)
                        HostKey(k.getString("keyType"), Base64.getDecoder().decode(k.getString("publicKey")))
                    }
                }
            }
        }
        return Vault(hosts.sortedBy { it.label.lowercase() }, keys, groups, snippets.sortedBy { it.name.lowercase() }, known,
            items.sortedWith(compareBy({ !it.favorite }, { it.name.lowercase() })))
    }

    // ------------------------------------------------------------ host keys trusted on this phone

    fun trustedHere(hostPort: String): List<HostKey> {
        if (!trustedFile.exists()) return emptyList()
        val arr = JSONObject(trustedFile.readText()).optJSONArray(hostPort) ?: return emptyList()
        return (0 until arr.length()).map {
            val o = arr.getJSONObject(it)
            HostKey(o.getString("type"), Base64.getDecoder().decode(o.getString("key")))
        }
    }

    fun trustHere(hostPort: String, key: HostKey) {
        val all = if (trustedFile.exists()) JSONObject(trustedFile.readText()) else JSONObject()
        all.put(hostPort, JSONArray().put(JSONObject().put("type", key.type).put("key", Base64.getEncoder().encodeToString(key.key))))
        writeAtomic(trustedFile, all.toString())
    }

    /** Signs out and deletes everything stored by the app. */
    fun erase() {
        lock()
        dir.deleteRecursively()
        dir.mkdirs()
    }

    // ------------------------------------------------------------ helpers

    private fun writeAtomic(f: File, text: String) {
        f.parentFile?.mkdirs()
        val tmp = File(f.parentFile, f.name + ".tmp")
        tmp.writeText(text)
        if (!tmp.renameTo(f)) {
            f.delete()
            tmp.renameTo(f)
        }
    }

    private fun keystoreKey(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .build())
        return gen.generateKey()
    }

    private fun sealToken(token: String): String {
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.ENCRYPT_MODE, keystoreKey())
        return Base64.getEncoder().encodeToString(c.iv + c.doFinal(token.toByteArray()))
    }

    private fun unsealToken(sealed: String): String {
        val raw = Base64.getDecoder().decode(sealed)
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.DECRYPT_MODE, keystoreKey(), GCMParameterSpec(128, raw, 0, 12))
        return String(c.doFinal(raw, 12, raw.size - 12))
    }

    companion object {
        private const val ALIAS = "remvyn-github-token"
        const val DEFAULT_ID = "default"
        private const val DEFAULT_NAME = "Default"
        private const val MAX_NAME = 60
    }
}
