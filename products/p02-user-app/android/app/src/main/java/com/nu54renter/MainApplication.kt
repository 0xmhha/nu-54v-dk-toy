package com.nu54renter

import android.app.Application
import com.facebook.react.PackageList
import com.facebook.react.ReactApplication
import com.facebook.react.ReactHost
import com.facebook.react.common.assets.ReactFontManager
import com.facebook.react.ReactNativeApplicationEntryPoint.loadReactNative
import com.facebook.react.defaults.DefaultReactHost.getDefaultReactHost
import com.nu54renter.ble.RenterPackage

class MainApplication : Application(), ReactApplication {

  override val reactHost: ReactHost by lazy {
    getDefaultReactHost(
      context = applicationContext,
      packageList =
        PackageList(this).packages.apply {
          add(RenterPackage())
        },
    )
  }

  override fun onCreate() {
    super.onCreate()
    // Design book typeface; fontWeight 400/700/800 selects the bundled file (res/font/pretendard.xml).
    ReactFontManager.getInstance().addCustomFont(this, "Pretendard", R.font.pretendard)
    loadReactNative(this)
  }
}
