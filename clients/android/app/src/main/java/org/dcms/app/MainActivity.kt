package org.dcms.app

import android.Manifest
import android.annotation.SuppressLint
import android.app.DownloadManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.net.http.SslError
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import android.view.ViewGroup
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.UUID
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.PermissionRequest
import android.webkit.SslErrorHandler
import android.webkit.URLUtil
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout

class MainActivity : AppCompatActivity() {

    private lateinit var webView: WebView
    private lateinit var swipeRefresh: SwipeRefreshLayout
    private var fileCallback: ValueCallback<Array<Uri>>? = null
    private var pendingPermRequest: PermissionRequest? = null
    private var lastBackAt = 0L

    private val prefs by lazy { getSharedPreferences("dcms_client", Context.MODE_PRIVATE) }

    private fun serverUrl(): String =
        prefs.getString("server_url", null)?.takeIf { it.isNotBlank() } ?: BuildConfig.SERVER_URL

    private fun serverHost(): String = try {
        Uri.parse(serverUrl()).host.orEmpty().lowercase()
    } catch (_: Exception) {
        ""
    }

    private val fileChooserLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { res ->
        val cb = fileCallback
        fileCallback = null
        if (cb == null) return@registerForActivityResult
        if (res.resultCode != RESULT_OK || res.data == null) {
            cb.onReceiveValue(null)
            return@registerForActivityResult
        }
        val data = res.data!!
        val clip = data.clipData
        val uris = when {
            clip != null && clip.itemCount > 0 -> Array(clip.itemCount) { i -> clip.getItemAt(i).uri }
            data.data != null -> arrayOf(data.data!!)
            else -> null
        }
        cb.onReceiveValue(uris)
    }

    private val mediaPermLauncher = registerForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions()
    ) { results ->
        val req = pendingPermRequest
        pendingPermRequest = null
        if (req != null) {
            val allGranted = results.values.all { it }
            if (allGranted) req.grant(req.resources) else req.deny()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        swipeRefresh = SwipeRefreshLayout(this).apply {
            setColorSchemeColors(Color.parseColor("#4f7cff"))
            setProgressBackgroundColorSchemeColor(Color.parseColor("#171c28"))
        }

        webView = WebView(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            )
            setBackgroundColor(Color.parseColor("#0f1219"))
        }

        swipeRefresh.addView(webView)
        setContentView(swipeRefresh)

        val cm = CookieManager.getInstance()
        cm.setAcceptCookie(true)
        cm.setAcceptThirdPartyCookies(webView, true)

        with(webView.settings) {
            javaScriptEnabled = true
            domStorageEnabled = true
            databaseEnabled = true
            allowFileAccess = true
            mediaPlaybackRequiresUserGesture = false
            mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
            cacheMode = WebSettings.LOAD_DEFAULT
            userAgentString = "$userAgentString DcmsAndroid/${BuildConfig.VERSION_NAME}"
        }

        webView.addJavascriptInterface(NativeBridge(), "dcmsNative")

        // Включаем pull-to-refresh только когда страница в самом верху
        webView.setOnScrollChangeListener { _, _, scrollY, _, _ ->
            swipeRefresh.isEnabled = scrollY == 0
        }
        swipeRefresh.setOnRefreshListener {
            if (webView.url.isNullOrBlank() || webView.url?.startsWith("data:") == true) {
                webView.loadUrl(serverUrl())
            } else {
                webView.reload()
            }
        }

        webView.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                val uri = request.url ?: return false
                val scheme = uri.scheme?.lowercase() ?: return false
                if (scheme != "http" && scheme != "https") {
                    try { startActivity(Intent(Intent.ACTION_VIEW, uri)) } catch (_: Exception) {}
                    return true
                }
                val host = uri.host?.lowercase().orEmpty()
                if (host.isNotEmpty() && host != serverHost()) {
                    try { startActivity(Intent(Intent.ACTION_VIEW, uri)) } catch (_: Exception) {}
                    return true
                }
                return false
            }

            override fun onPageFinished(view: WebView, url: String?) {
                swipeRefresh.isRefreshing = false
                CookieManager.getInstance().flush()
            }

            override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
                if (request.isForMainFrame) {
                    swipeRefresh.isRefreshing = false
                    showOfflinePage()
                }
            }

            @SuppressLint("WebViewClientOnReceivedSslError")
            override fun onReceivedSslError(view: WebView, handler: SslErrorHandler, error: SslError) {
                if (BuildConfig.ALLOW_INSECURE_TLS) {
                    handler.proceed()
                } else {
                    handler.cancel()
                }
            }
        }

        webView.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(
                webView: WebView,
                filePathCallback: ValueCallback<Array<Uri>>,
                fileChooserParams: FileChooserParams
            ): Boolean {
                fileCallback?.onReceiveValue(null)
                fileCallback = filePathCallback
                val intent = try {
                    fileChooserParams.createIntent().apply {
                        putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                    }
                } catch (_: Exception) {
                    Intent(Intent.ACTION_GET_CONTENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        type = "*/*"
                        putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                    }
                }
                fileChooserLauncher.launch(intent)
                return true
            }

            override fun onPermissionRequest(request: PermissionRequest) {
                val needed = mutableListOf<String>()
                if (request.resources.contains(PermissionRequest.RESOURCE_AUDIO_CAPTURE) &&
                    ContextCompat.checkSelfPermission(this@MainActivity, Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED
                ) {
                    needed.add(Manifest.permission.RECORD_AUDIO)
                }
                if (request.resources.contains(PermissionRequest.RESOURCE_VIDEO_CAPTURE) &&
                    ContextCompat.checkSelfPermission(this@MainActivity, Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED
                ) {
                    needed.add(Manifest.permission.CAMERA)
                }
                if (needed.isNotEmpty()) {
                    pendingPermRequest = request
                    mediaPermLauncher.launch(needed.toTypedArray())
                } else {
                    request.grant(request.resources)
                }
            }
        }

        webView.setDownloadListener { url, userAgent, contentDisposition, mimeType, _ ->
            try {
                val req = DownloadManager.Request(Uri.parse(url)).apply {
                    setMimeType(mimeType)
                    val cookies = CookieManager.getInstance().getCookie(url)
                    if (!cookies.isNullOrEmpty()) addRequestHeader("Cookie", cookies)
                    addRequestHeader("User-Agent", userAgent)
                    val filename = URLUtil.guessFileName(url, contentDisposition, mimeType)
                    setTitle(filename)
                    setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                    setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, filename)
                }
                val dm = getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager
                dm.enqueue(req)
                Toast.makeText(this, "Загрузка файла начата…", Toast.LENGTH_SHORT).show()
            } catch (_: Exception) {
                try { startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) } catch (_: Exception) {}
            }
        }

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (webView.canGoBack()) {
                    webView.goBack()
                } else {
                    val now = System.currentTimeMillis()
                    if (now - lastBackAt < 1800) {
                        finish()
                    } else {
                        lastBackAt = now
                        Toast.makeText(this@MainActivity, "Нажмите «Назад» ещё раз для выхода", Toast.LENGTH_SHORT).show()
                    }
                }
            }
        })

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 101)
        }

        if (savedInstanceState != null) {
            webView.restoreState(savedInstanceState)
        } else {
            webView.loadUrl(serverUrl())
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        webView.saveState(outState)
    }

    private fun showOfflinePage() {
        val url = serverUrl()
        val html = """
            <!doctype html><html lang="ru"><head><meta charset="utf-8">
            <meta name="viewport" content="width=device-width,initial-scale=1">
            <style>
              body{margin:0;min-height:100vh;display:grid;place-items:center;padding:20px;
                font-family:system-ui,sans-serif;background:#0f1219;color:#eef2ff}
              .c{width:100%;max-width:380px;padding:24px;border-radius:16px;background:#171c28;border:1px solid #262e42}
              h2{margin:0 0 8px}p{margin:0 0 16px;color:#9aa6c1;font-size:14px}
              input{width:100%;box-sizing:border-box;padding:11px;border-radius:10px;border:1px solid #2d3750;
                background:#0f131d;color:#fff;margin-bottom:12px;font-size:15px}
              .r{display:flex;gap:8px}
              button{flex:1;padding:11px;border:0;border-radius:10px;font-weight:600;font-size:15px}
              .p{background:#4f7cff;color:#fff}.g{background:#232b3d;color:#dbe4ff}
            </style></head><body><div class="c">
              <h2>Нет подключения</h2>
              <p>Не удалось открыть сервер <b>$url</b>. Проверьте сеть или адрес.</p>
              <input id="u" value="$url" type="url">
              <div class="r">
                <button class="p" onclick="dcmsNative.retry()">Повторить</button>
                <button class="g" onclick="dcmsNative.setServerUrl(document.getElementById('u').value)">Сохранить</button>
              </div>
            </div></body></html>
        """.trimIndent()
        webView.loadDataWithBaseURL(null, html, "text/html", "utf-8", null)
    }

    private fun getOrCreateDeviceId(): String {
        val cur = prefs.getString("mfa_device_id", null)
        if (!cur.isNullOrBlank()) return cur
        val id = "and-" + UUID.randomUUID().toString()
        prefs.edit().putString("mfa_device_id", id).apply()
        return id
    }

    private fun ensureKeystoreECKey(): KeyStore.PrivateKeyEntry {
        val alias = "dcms_2fa_ec_v1"
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (!ks.containsAlias(alias)) {
            val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
            val spec = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256)
                .build()
            kpg.initialize(spec)
            kpg.generateKeyPair()
        }
        return ks.getEntry(alias, null) as KeyStore.PrivateKeyEntry
    }

    inner class NativeBridge {
        @JavascriptInterface
        fun platform(): String = "android"

        @JavascriptInterface
        fun canSign2FA(): Boolean = true

        @JavascriptInterface
        fun getDeviceId(): String = getOrCreateDeviceId()

        @JavascriptInterface
        fun getDeviceName(): String {
            val mfg = Build.MANUFACTURER.orEmpty().trim()
            val mod = Build.MODEL.orEmpty().trim()
            val title = if (mod.startsWith(mfg, ignoreCase = true)) mod else "$mfg $mod".trim()
            return "$title (Android ${Build.VERSION.RELEASE})".trim()
        }

        @JavascriptInterface
        fun ensure2FAKey(): String {
            val entry = ensureKeystoreECKey()
            return Base64.encodeToString(entry.certificate.publicKey.encoded, Base64.NO_WRAP)
        }

        @JavascriptInterface
        fun sign2FA(payload: String): String {
            val entry = ensureKeystoreECKey()
            val sig = Signature.getInstance("SHA256withECDSA")
            sig.initSign(entry.privateKey)
            sig.update(payload.toByteArray(Charsets.UTF_8))
            return Base64.encodeToString(sig.sign(), Base64.NO_WRAP)
        }

        @JavascriptInterface
        fun getServerUrl(): String = serverUrl()

        @JavascriptInterface
        fun setServerUrl(url: String) {
            var u = url.trim().trimEnd('/')
            if (u.isEmpty()) return
            if (!u.startsWith("http://") && !u.startsWith("https://")) u = "https://$u"
            prefs.edit().putString("server_url", u).apply()
            runOnUiThread { webView.loadUrl(u) }
        }

        @JavascriptInterface
        fun retry() {
            runOnUiThread { webView.loadUrl(serverUrl()) }
        }
    }
}
