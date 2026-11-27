package com.nu54renter.ble

import com.facebook.react.BaseReactPackage
import com.facebook.react.bridge.NativeModule
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.module.model.ReactModuleInfo
import com.facebook.react.module.model.ReactModuleInfoProvider

/** Registers the renter app's own Turbo Modules (they are not autolinked). */
class RenterPackage : BaseReactPackage() {
  override fun getModule(name: String, context: ReactApplicationContext): NativeModule? =
    when (name) {
      NativeMarketLabSpec.NAME -> MarketLabModule(context)
      NativeRenterBleSpec.NAME -> RenterBleModule(context)
      NativeCloudWalletProfileSpec.NAME -> CloudWalletProfileModule(context)
      NativeDappBrowserSpec.NAME -> DappBrowserModule(context)
      NativeOwnerWalletSpec.NAME -> OwnerWalletModule(context)
      NativeNearbySpec.NAME -> NearbyModule(context)
      NativeShopQrSpec.NAME -> ShopQrModule(context)
      NativeCommerceHttpSpec.NAME -> CommerceHttpModule(context)
      NativeSubscriptionAlertsSpec.NAME -> SubscriptionAlertsModule(context)
      else -> null
    }

  override fun getReactModuleInfoProvider() = ReactModuleInfoProvider {
    mapOf(
      NativeCloudWalletProfileSpec.NAME to ReactModuleInfo(
        NativeCloudWalletProfileSpec.NAME, NativeCloudWalletProfileSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeDappBrowserSpec.NAME to ReactModuleInfo(
        NativeDappBrowserSpec.NAME, NativeDappBrowserSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeSubscriptionAlertsSpec.NAME to ReactModuleInfo(
        NativeSubscriptionAlertsSpec.NAME, NativeSubscriptionAlertsSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeShopQrSpec.NAME to ReactModuleInfo(
        NativeShopQrSpec.NAME, NativeShopQrSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeNearbySpec.NAME to ReactModuleInfo(
        NativeNearbySpec.NAME, NativeNearbySpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeCommerceHttpSpec.NAME to ReactModuleInfo(
        NativeCommerceHttpSpec.NAME, NativeCommerceHttpSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeOwnerWalletSpec.NAME to ReactModuleInfo(
        NativeOwnerWalletSpec.NAME, NativeOwnerWalletSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeMarketLabSpec.NAME to ReactModuleInfo(
        NativeMarketLabSpec.NAME, NativeMarketLabSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
      NativeRenterBleSpec.NAME to ReactModuleInfo(
        NativeRenterBleSpec.NAME, NativeRenterBleSpec.NAME,
        canOverrideExistingModule = false, needsEagerInit = false, isCxxModule = false, isTurboModule = true,
      ),
    )
  }
}
