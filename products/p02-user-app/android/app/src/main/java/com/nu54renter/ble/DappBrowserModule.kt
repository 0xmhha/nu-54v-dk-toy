package com.nu54renter.ble

import android.app.AlertDialog
import android.app.Dialog
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.view.ViewGroup
import android.webkit.CookieManager
import android.webkit.WebMessage
import android.webkit.WebMessagePort
import android.webkit.WebResourceRequest
import android.webkit.WebChromeClient
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.LinearLayout
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import org.json.JSONObject
import java.net.URI
import java.util.UUID

class DappBrowserModule(context: ReactApplicationContext): NativeDappBrowserSpec(context) {
  private val main = Handler(Looper.getMainLooper())
  private var dialog: Dialog? = null
  private var page: WebView? = null
  private var port: WebMessagePort? = null
  private var channel = ""
  private var document = ""

  private fun origin(url: String): String? = runCatching {
    val uri = URI(url)
    require(uri.scheme == "https" && uri.host != null && uri.userInfo == null)
    val host = uri.host.lowercase()
    require(host.none { it == ':' || it == '[' || it == ']' })
    "https://$host" + if (uri.port == -1 || uri.port == 443) "" else ":${uri.port}"
  }.getOrNull()

  private fun reset() {
    port?.close()
    port = null
    page?.stopLoading()
    page?.destroy()
    page = null
    dialog?.setOnDismissListener(null)
    dialog?.dismiss()
    dialog = null
    val old = channel
    channel = ""
    document = ""
    if (old.isNotEmpty()) emitOnClosed(old)
  }

  override fun open(url: String, session: String, provider: String, promise: Promise) {
    main.post {
      val activity = reactApplicationContext.currentActivity
      val expected = origin(url)
      if (activity == null || expected == null || !session.matches(Regex("[a-f0-9]{64}")) || provider.length !in 1..8192) {
        promise.reject("DAPP_BROWSER", "The site could not be opened")
        return@post
      }
      reset()
      channel = session
      val browser = WebView(activity)
      page = browser
      with(browser.settings) {
        javaScriptEnabled = true
        allowFileAccess = false
        allowContentAccess = false
        domStorageEnabled = false
        mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_NEVER_ALLOW
        setSupportMultipleWindows(true)
        javaScriptCanOpenWindowsAutomatically = false
        setGeolocationEnabled(false)
      }
      CookieManager.getInstance().setAcceptCookie(false)
      CookieManager.getInstance().setAcceptThirdPartyCookies(browser, false)
      browser.webChromeClient = object: WebChromeClient() {
        override fun onCreateWindow(view: WebView, isDialog: Boolean, isUserGesture: Boolean, resultMsg: android.os.Message): Boolean =
          false
      }
      browser.webViewClient = object: WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
          val unrestricted = origin(request.url.toString()) == expected
          if (request.isForMainFrame && (!unrestricted || document.isNotEmpty())) {
            reset()
            return true
          }
          return !unrestricted
        }

        override fun onPageStarted(view: WebView, address: String, favicon: android.graphics.Bitmap?) {
          if (document.isNotEmpty() || origin(address) != expected) reset()
        }

        override fun onPageFinished(view: WebView, address: String) {
          if (page !== view || channel != session || origin(address) != expected || port != null) return
          val pair = view.createWebMessageChannel()
          port = pair[0]
          val currentDocument = UUID.randomUUID().toString()
          document = currentDocument
          pair[0].setWebMessageCallback(object: WebMessagePort.WebMessageCallback() {
            override fun onMessage(sender: WebMessagePort, message: WebMessage) {
              val body = message.data ?: return
              if (page !== view || port !== sender || channel != session ||
                origin(view.url.orEmpty()) != expected || body.length !in 2..4096) return
              emitOnRequest(JSONObject().put("session",session).put("origin",expected).put("document",currentDocument).put("request",body).toString())
            }
          })
          view.evaluateJavascript(provider) {
            if (page === view && channel == session && port === pair[0] && origin(view.url.orEmpty()) == expected)
              view.postWebMessage(WebMessage("Nu54WalletProvider", arrayOf(pair[1])), Uri.parse(expected))
            else pair[1].close()
          }
        }
      }
      val layout = LinearLayout(activity).apply { orientation = LinearLayout.VERTICAL }
      layout.addView(Button(activity).apply {
        text = "Close · $expected"
        setOnClickListener { reset() }
      })
      layout.addView(browser, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT,0,1f))
      val presentation=Dialog(activity,android.R.style.Theme_Material_Light_NoActionBar)
      dialog=presentation
      presentation.setContentView(layout)
      presentation.setOnDismissListener { if (dialog === presentation) reset() }
      presentation.show()
      browser.loadUrl(url)
      promise.resolve(null)
    }
  }

  override fun respond(session: String, document: String, reply: String, promise: Promise) {
    main.post {
      val recipient = port
      if (session != channel || document != this.document || recipient == null || reply.length !in 2..4096) {
        promise.reject("DAPP_CHANNEL", "The wallet channel is closed")
        return@post
      }
      try { recipient.postMessage(WebMessage(reply)); promise.resolve(null) }
      catch (_: IllegalStateException) { promise.reject("DAPP_CHANNEL", "The wallet channel is closed") }
    }
  }

  override fun confirm(session: String, document: String, title: String, message: String, promise: Promise) {
    main.post {
      val activity = reactApplicationContext.currentActivity
      if (activity == null || session != channel || document != this.document || title.length > 120 || message.length > 500) {
        promise.resolve(false)
        return@post
      }
      AlertDialog.Builder(activity).setTitle(title).setMessage(message)
        .setNegativeButton("Cancel") { _, _ -> promise.resolve(false) }
        .setPositiveButton("Ask board") { _, _ -> promise.resolve(session == channel && document == this.document) }
        .setOnCancelListener { promise.resolve(false) }.show()
    }
  }

  override fun confirmAllowance(session: String, document: String, title: String, message: String, promise: Promise) {
    main.post {
      val activity = reactApplicationContext.currentActivity
      fun current() = session == channel && document == this.document
      val login = title.startsWith("사이트 로그인 · ")
      val messageLimit = if (login) 1000 else 500
      if (activity == null || !current() || title.length > 120 || message.length > messageLimit) {
        promise.resolve("reject")
        return@post
      }
      AlertDialog.Builder(activity).setTitle(title).setMessage(message)
        .setNegativeButton("거절") { _, _ -> promise.resolve("reject") }
        .setNeutralButton(if (login) "세션 보기" else "연결 보기") { _, _ -> promise.resolve(if (current()) "connections" else "reject") }
        .setPositiveButton("보드 요청") { _, _ -> promise.resolve(if (current()) "approve" else "reject") }
        .setOnCancelListener { promise.resolve("reject") }.show()
    }
  }

  override fun close(promise: Promise) {
    main.post { reset(); promise.resolve(null) }
  }

  override fun invalidate() {
    main.post { reset() }
    super.invalidate()
  }
}
