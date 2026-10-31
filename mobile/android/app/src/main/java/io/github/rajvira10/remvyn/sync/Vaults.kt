package io.github.rajvira10.remvyn.sync

import android.content.Context
import org.json.JSONObject
import java.io.File
import java.util.UUID

/** A vault downloaded to this phone. */
data class VaultInfo(val id: String, val repo: String)

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
            if (s.hasVault()) VaultInfo(id, s.config().repo) else null
        }.let { l -> l.take(1) + l.drop(1).sortedBy { it.repo.lowercase() } }
    }

    /** The vault the app opens with. */
    var current: String
        get() = runCatching { JSONObject(stateFile.readText()).optString("current") }.getOrNull()?.ifEmpty { null } ?: DEFAULT
        set(id) = stateFile.writeText(JSONObject().put("current", id).toString())

    /** A folder for a vault being added; [VaultStore.hasVault] is false until it's downloaded. */
    fun newId(): String = UUID.randomUUID().toString().replace("-", "").take(12)

    /** Deletes a vault's folder. */
    fun remove(id: String) = store(id).erase()

    /** Leftovers of vaults whose first download never finished. */
    fun cleanUp() {
        File(root, "vaults").listFiles()?.filter { it.isDirectory && !store(it.name).hasVault() }?.forEach { it.deleteRecursively() }
    }

    companion object {
        const val DEFAULT = "default"
    }
}
