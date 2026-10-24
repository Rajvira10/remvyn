package io.github.rajvira10.remvyn.sync

import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLEncoder
import java.util.Base64

class GitHubException(message: String, val code: String = "", val status: Int = 0) : Exception(message)

data class DeviceCode(val deviceCode: String, val userCode: String, val verificationUri: String, val expiresIn: Int, val interval: Int)

data class Repo(val fullName: String, val private: Boolean, val defaultBranch: String)

/** The pieces of GitHub that Remvyn sync uses (same endpoints as the desktop). */
class GitHub(val web: String, val api: String, val clientId: String, var token: String = "") {

    private fun open(url: String, method: String): HttpURLConnection {
        val c = URL(url).openConnection() as HttpURLConnection
        c.requestMethod = method
        c.connectTimeout = 20_000
        c.readTimeout = 30_000
        return c
    }

    private fun readBody(c: HttpURLConnection): String {
        val stream = if (c.responseCode >= 400) c.errorStream else c.inputStream
        return stream?.bufferedReader()?.use { it.readText() } ?: ""
    }

    private fun oauth(path: String, form: Map<String, String>): JSONObject {
        val c = open(web + path, "POST")
        c.doOutput = true
        c.setRequestProperty("Accept", "application/json")
        c.setRequestProperty("Content-Type", "application/x-www-form-urlencoded")
        val body = form.entries.joinToString("&") { "${it.key}=${URLEncoder.encode(it.value, "UTF-8")}" }
        try {
            c.outputStream.use { it.write(body.toByteArray()) }
            if (c.responseCode == 404) throw GitHubException("GitHub didn't recognise the app's client ID", "github_not_set_up", 404)
            val text = readBody(c)
            if (c.responseCode >= 300) throw GitHubException("GitHub sign-in failed (${c.responseCode})", status = c.responseCode)
            return JSONObject(text)
        } catch (e: IOException) {
            throw GitHubException("Can't reach GitHub (${e.message})")
        } finally {
            c.disconnect()
        }
    }

    fun startDeviceFlow(): DeviceCode {
        if (clientId.isBlank()) throw GitHubException("Set up the Remvyn GitHub app first", "github_not_set_up")
        val r = oauth("/login/device/code", mapOf("client_id" to clientId))
        r.optString("error").takeIf { it.isNotEmpty() }?.let { throw deviceError(it, r.optString("error_description")) }
        return DeviceCode(r.getString("device_code"), r.getString("user_code"), r.getString("verification_uri"),
            r.optInt("expires_in", 900), r.optInt("interval", 5).coerceAtLeast(1))
    }

    private fun deviceError(code: String, desc: String) = when (code) {
        "access_denied" -> GitHubException("Sign-in was cancelled on GitHub", "github_denied")
        "expired_token", "token_expired" -> GitHubException("The sign-in code expired; start again", "github_code_expired")
        "device_flow_disabled" -> GitHubException("Device flow is turned off for this GitHub app", "github_device_flow")
        "incorrect_client_credentials" -> GitHubException("GitHub didn't recognise the app's client ID", "github_not_set_up")
        else -> GitHubException("GitHub sign-in failed: ${desc.ifEmpty { code }}")
    }

    /** Polls until the user approves; call from a background thread. */
    fun pollToken(dc: DeviceCode, cancelled: () -> Boolean): String {
        var interval = dc.interval * 1000L
        val deadline = System.currentTimeMillis() + dc.expiresIn * 1000L
        while (!cancelled()) {
            Thread.sleep(interval)
            if (cancelled()) break
            if (System.currentTimeMillis() > deadline) throw deviceError("expired_token", "")
            val r = oauth("/login/oauth/access_token", mapOf(
                "client_id" to clientId,
                "device_code" to dc.deviceCode,
                "grant_type" to "urn:ietf:params:oauth:grant-type:device_code",
            ))
            when (val err = r.optString("error")) {
                "" -> return r.getString("access_token").also { token = it }
                "authorization_pending" -> {}
                "slow_down" -> interval = r.optInt("interval", (interval / 1000 + 5).toInt()) * 1000L
                else -> throw deviceError(err, r.optString("error_description"))
            }
        }
        throw GitHubException("Sign-in cancelled", "cancelled")
    }

    private fun get(path: String): Pair<Int, String> {
        val c = open(api + path, "GET")
        c.setRequestProperty("Accept", "application/vnd.github+json")
        c.setRequestProperty("X-GitHub-Api-Version", "2022-11-28")
        if (token.isNotEmpty()) c.setRequestProperty("Authorization", "Bearer $token")
        try {
            val code = c.responseCode
            val body = readBody(c)
            if (code == 401) throw GitHubException("GitHub sign-in expired or was revoked; sign in again", "github_auth", 401)
            if ((code == 403 || code == 429) && c.getHeaderField("X-RateLimit-Remaining") == "0") {
                throw GitHubException("GitHub rate limit reached; try again later", "github_rate_limited", code)
            }
            return code to body
        } catch (e: IOException) {
            throw GitHubException("Can't reach GitHub (${e.message})")
        } finally {
            c.disconnect()
        }
    }

    private fun getJson(path: String): JSONObject {
        val (code, body) = get(path)
        if (code >= 300) {
            val msg = runCatching { JSONObject(body).optString("message") }.getOrNull().orEmpty()
            throw GitHubException("GitHub: ${msg.ifEmpty { "error $code" }}", status = code)
        }
        return JSONObject(body)
    }

    fun user(): String = getJson("/user").getString("login")

    /** Repositories the app is installed on; null when the app isn't installed. */
    fun accessibleRepos(): List<Repo>? {
        val inst = getJson("/user/installations?per_page=100").getJSONArray("installations")
        if (inst.length() == 0) return null
        val out = mutableListOf<Repo>()
        for (i in 0 until inst.length()) {
            val id = inst.getJSONObject(i).getLong("id")
            var page = 1
            while (page <= 10) {
                val arr = getJson("/user/installations/$id/repositories?per_page=100&page=$page").getJSONArray("repositories")
                for (j in 0 until arr.length()) out += repo(arr.getJSONObject(j))
                if (arr.length() < 100) break
                page++
            }
        }
        return out
    }

    private fun repo(o: JSONObject) = Repo(o.getString("full_name"), o.optBoolean("private"), o.optString("default_branch", "main"))

    fun getRepo(fullName: String): Repo {
        val (code, body) = get("/repos/$fullName")
        if (code == 404) throw GitHubException("Remvyn can't see $fullName; give the Remvyn GitHub app access to it", "github_no_access", 404)
        if (code >= 300) throw GitHubException("GitHub: error $code", status = code)
        return repo(JSONObject(body))
    }

    /** Head commit of the branch, or null for an empty repository. */
    fun ref(fullName: String, branch: String): String? {
        val (code, body) = get("/repos/$fullName/git/ref/heads/$branch")
        if (code == 409 || code == 404) return null
        if (code >= 300) throw GitHubException("GitHub: error $code", status = code)
        return JSONObject(body).getJSONObject("object").getString("sha")
    }

    /** path -> blob sha for every file at commit. */
    fun files(fullName: String, commit: String): Map<String, String> {
        val tree = getJson("/repos/$fullName/git/commits/$commit").getJSONObject("tree").getString("sha")
        val t = getJson("/repos/$fullName/git/trees/$tree?recursive=1")
        if (t.optBoolean("truncated")) throw GitHubException("The vault repository is too large to list")
        val arr: JSONArray = t.getJSONArray("tree")
        val out = HashMap<String, String>()
        for (i in 0 until arr.length()) {
            val e = arr.getJSONObject(i)
            if (e.getString("type") == "blob") out[e.getString("path")] = e.getString("sha")
        }
        return out
    }

    fun blob(fullName: String, sha: String): String {
        val b = getJson("/repos/$fullName/git/blobs/$sha")
        val content = b.getString("content")
        return if (b.optString("encoding") == "base64") String(Base64.getMimeDecoder().decode(content), Charsets.UTF_8) else content
    }
}
