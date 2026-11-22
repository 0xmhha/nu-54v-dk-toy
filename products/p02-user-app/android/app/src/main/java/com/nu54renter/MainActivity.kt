package com.nu54renter

import com.facebook.react.ReactActivity
import android.os.Build
import android.graphics.Color
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import com.facebook.react.ReactActivityDelegate
import com.facebook.react.defaults.DefaultNewArchitectureEntryPoint.fabricEnabled
import com.facebook.react.defaults.DefaultReactActivityDelegate

class MainActivity : ReactActivity() {
  private var homeProtected = false
  private var secureAlreadySet = false
  private var previousAccessibility: Int? = null
  private var privacyCover: View? = null
  private var resumed = false
  private var focused = false

  fun setWalletHomeProtection(enabled: Boolean) {
    if (enabled == homeProtected) return
    if (enabled) {
      secureAlreadySet = (window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE) != 0
      window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
      if (Build.VERSION.SDK_INT >= 33) setRecentsScreenshotEnabled(false)
      if (!focused) { showPrivacyCover(); hideWalletAccessibility() }
    } else {
      if (!secureAlreadySet) window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
      if (Build.VERSION.SDK_INT >= 33 && resumed && focused) setRecentsScreenshotEnabled(true)
      if (resumed && focused) { removePrivacyCover(); restoreWalletAccessibility() }
    }
    homeProtected = enabled
  }

  private fun hideWalletAccessibility() {
    if (previousAccessibility != null) return
    previousAccessibility = window.decorView.importantForAccessibility
    window.decorView.importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS
  }

  private fun restoreWalletAccessibility() {
    previousAccessibility?.let { window.decorView.importantForAccessibility = it }
    previousAccessibility = null
  }

  private fun showPrivacyCover() {
    if (privacyCover != null) return
    val content = findViewById<ViewGroup>(android.R.id.content) ?: return
    val cover = View(this).apply {
      setBackgroundColor(Color.WHITE)
      elevation = Float.MAX_VALUE
      importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
    }
    content.addView(cover, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
    cover.bringToFront()
    privacyCover = cover
  }

  private fun removePrivacyCover() {
    privacyCover?.let { (it.parent as? ViewGroup)?.removeView(it) }
    privacyCover = null
  }

  override fun onPause() {
    resumed = false
    if (homeProtected) showPrivacyCover()
    if (homeProtected) hideWalletAccessibility()
    super.onPause()
  }

  override fun onResume() {
    super.onResume()
    resumed = true
    if (focused) {
      if (!homeProtected && Build.VERSION.SDK_INT >= 33) setRecentsScreenshotEnabled(true)
      removePrivacyCover()
      restoreWalletAccessibility()
    }
  }

  override fun onWindowFocusChanged(hasFocus: Boolean) {
    super.onWindowFocusChanged(hasFocus)
    focused = hasFocus
    if (!hasFocus && homeProtected) {
      showPrivacyCover()
      hideWalletAccessibility()
    } else if (hasFocus && resumed) {
      if (!homeProtected && Build.VERSION.SDK_INT >= 33) setRecentsScreenshotEnabled(true)
      removePrivacyCover()
      restoreWalletAccessibility()
    }
  }

  /**
   * Returns the name of the main component registered from JavaScript. This is used to schedule
   * rendering of the component.
   */
  override fun getMainComponentName(): String = "NU54Renter"

  /**
   * Returns the instance of the [ReactActivityDelegate]. We use [DefaultReactActivityDelegate]
   * which allows you to enable New Architecture with a single boolean flags [fabricEnabled]
   */
  override fun createReactActivityDelegate(): ReactActivityDelegate =
      DefaultReactActivityDelegate(this, mainComponentName, fabricEnabled)
}
