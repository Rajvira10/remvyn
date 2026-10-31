package io.github.rajvira10.remvyn

import io.github.rajvira10.remvyn.sync.SyncConfig
import io.github.rajvira10.remvyn.sync.Vaults
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class VaultsTest {
    @get:Rule val tmp = TemporaryFolder()

    /** Makes a vault look downloaded: a config naming the repository and a cache file. */
    private fun download(v: Vaults, id: String, repo: String) {
        val s = v.store(id)
        s.saveConfig(SyncConfig(repo = repo))
        File(s.dir, "cache.json").writeText("{}")
    }

    @Test fun listsDownloadedVaultsDefaultFirst() {
        val root = tmp.newFolder()
        val v = Vaults(root)
        assertEquals(emptyList<Any>(), v.list())
        download(v, Vaults.DEFAULT, "me/remvyn-vault")
        download(v, "b", "me/zeta")
        download(v, "a", "me/alpha")
        // Started but never downloaded: not listed.
        v.store("half").saveConfig(SyncConfig(repo = "me/half"))
        assertEquals(listOf("me/remvyn-vault", "me/alpha", "me/zeta"), v.list().map { it.repo })
        // The first vault keeps the folder older versions used.
        assertTrue(File(root, "vault/cache.json").exists())
        assertTrue(File(root, "vaults/a/cache.json").exists())
    }

    @Test fun remembersTheCurrentVault() {
        val root = tmp.newFolder()
        assertEquals(Vaults.DEFAULT, Vaults(root).current)
        Vaults(root).current = "a"
        assertEquals("a", Vaults(root).current)
    }

    @Test fun removesAndCleansUp() {
        val root = tmp.newFolder()
        val v = Vaults(root)
        download(v, Vaults.DEFAULT, "me/one")
        download(v, "a", "me/two")
        v.store("half").saveConfig(SyncConfig(repo = "me/half"))
        v.cleanUp()
        assertFalse(File(root, "vaults/half").exists())
        assertTrue(File(root, "vaults/a").exists())
        v.remove("a")
        assertEquals(listOf("me/one"), v.list().map { it.repo })
        v.remove(Vaults.DEFAULT)
        assertEquals(emptyList<Any>(), v.list())
    }
}
