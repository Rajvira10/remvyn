package io.github.rajvira10.remvyn.sync

import android.content.Context
import org.json.JSONObject
import java.io.File
import java.util.UUID

/** A vault downloaded to this phone. */
data class VaultInfo(val id: String, val name: String, val repo: String)

/**
 * The vaults on this phone, like Settings → Vaults on the desktop: each one
 * is a repository with its own master password, kept in its own folder, and
 * all share one GitHub sign-in. The first vault keeps the original folder.
 */
class Vaults(private val root: File) {
    constructor(ctx: Context) : this(ctx.filesDir)

    private val stateFile = File(root, "vaults.json")

    private fun dirOf(id: String) = if (id == DEFAULT) File(root, "vault") else File(root, "vaults/$id")

    fun store(id: String) = VaultStore(dirOf(id))

    /** Downloaded vaults, the first one first, then by repository. */
    fun list(): List<VaultInfo> {
        val ids = listOf(DEFAULT) + (File(root, "vaults").listFiles()?.filter { it.isDirectory }?.map { it.name }?.sorted() ?: emptyList())
        return ids.mapNotNull { id ->
            val s = store(id)
            if (!s.hasVault()) return@mapNotNull null
            val repo = s.config().repo
            VaultInfo(id, names.optString(id).ifEmpty { if (id == DEFAULT) "Default" else repo.substringAfter('/') }, repo)
        }.let { l -> l.take(1) + l.drop(1).sortedBy { it.name.lowercase() } }
    }

    // vaults.json: {"current": id, "names": {id: name}}
    private fun state(): JSONObject = runCatching { JSONObject(stateFile.readText()) }.getOrDefault(JSONObject())

    private fun saveState(o: JSONObject) {
        val tmp = File(root, "vaults.json.tmp")
        tmp.writeText(o.toString())
        if (!tmp.renameTo(stateFile)) {
            stateFile.delete()
            tmp.renameTo(stateFile)
        }
    }

    private val names get() = state().optJSONObject("names") ?: JSONObject()

    /** The vault the app opens with. */
    var current: String
        get() = state().optString("current").ifEmpty { DEFAULT }
        set(id) = saveState(state().put("current", id))

    fun rename(id: String, name: String) {
        val n = name.trim().take(MAX_NAME)
        if (n.isEmpty()) return
        val o = state()
        o.put("names", (o.optJSONObject("names") ?: JSONObject()).put(id, n))
        saveState(o)
    }

    /** A folder for a vault being added; [VaultStore.hasVault] is false until it's downloaded. */
    fun newId(): String = UUID.randomUUID().toString().replace("-", "").take(12)

    /** Deletes a vault's folder and name. */
    fun remove(id: String) {
        store(id).erase()
        val o = state()
        o.optJSONObject("names")?.remove(id)
        saveState(o)
    }

    /** Leftovers of vaults whose first download never finished. */
    fun cleanUp() {
        File(root, "vaults").listFiles()?.filter { it.isDirectory && !store(it.name).hasVault() }?.forEach { it.deleteRecursively() }
    }

    companion object {
        const val DEFAULT = "default"
        const val MAX_NAME = 60
    }
}
