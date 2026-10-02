package com.nu54renter.ble

import com.facebook.react.BaseReactPackage
import com.facebook.react.bridge.NativeModule
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.module.model.ReactModuleInfo
import com.facebook.react.module.model.ReactModuleInfoProvider

/** Registers the renter app's own Turbo Modules (they are not autolinked). */
class RenterPackage : BaseReactPackage() {
  override fun getModule(name: String, context: ReactApplicationContext): NativeModule? =
    if (name == NativeRenterBleSpec.NAME) RenterBleModule(context) else null

  override fun getReactModuleInfoProvider() = ReactModuleInfoProvider {
    mapOf(
      NativeRenterBleSpec.NAME to ReactModuleInfo(
        NativeRenterBleSpec.NAME, NativeRenterBleSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
    )
  }
}
