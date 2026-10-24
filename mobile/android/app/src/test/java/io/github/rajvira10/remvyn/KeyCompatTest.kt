package io.github.rajvira10.remvyn

import com.jcraft.jsch.JSch
import com.jcraft.jsch.KeyPair
import org.json.JSONArray
import org.junit.Assert.assertEquals
import org.junit.Test
import java.util.Base64

/** JSch can use every kind of key the desktop generates (backend/keys/fixture_test.go). */
class KeyCompatTest {
    @Test
    fun loadsDesktopKeys() {
        val keys = JSONArray(javaClass.getResource("/keys-fixture.json")!!.readText())
        for (i in 0 until keys.length()) {
            val k = keys.getJSONObject(i)
            val jsch = JSch()
            val pass = k.getString("passphrase").takeIf { it.isNotEmpty() }?.toByteArray()
            jsch.addIdentity("k$i", k.getString("pem").toByteArray(), null, pass)
            val kp = KeyPair.load(jsch, k.getString("pem").toByteArray(), null)
            if (pass != null) kp.decrypt(pass)
            val expected = k.getString("public").trim().split(" ")[1]
            assertEquals("${k.getString("type")} public key", expected, Base64.getEncoder().encodeToString(kp.publicKeyBlob))
        }
    }
}
