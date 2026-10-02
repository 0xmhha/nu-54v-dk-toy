package com.nu54kiosk.ble

import com.facebook.react.BaseReactPackage
import com.facebook.react.bridge.NativeModule
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.module.model.ReactModuleInfo
import com.facebook.react.module.model.ReactModuleInfoProvider

/** Registers the kiosk's own Turbo Modules (they are not autolinked). */
class KioskPackage : BaseReactPackage() {
  override fun getModule(name: String, context: ReactApplicationContext): NativeModule? =
    when (name) {
      NativeNusBleSpec.NAME -> NusBleModule(context)
      NativeKioskVaultSpec.NAME -> KioskVaultModule(context)
      else -> null
    }

  override fun getReactModuleInfoProvider() = ReactModuleInfoProvider {
    listOf(NativeNusBleSpec.NAME, NativeKioskVaultSpec.NAME).associateWith {
      ReactModuleInfo(it, it, canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true)
    }
  }
}
