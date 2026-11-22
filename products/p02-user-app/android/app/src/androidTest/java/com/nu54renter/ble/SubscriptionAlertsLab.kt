package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.app.Notification
import android.app.NotificationManager
import android.os.Bundle

/** Local integration probe for the real Android schedule, broadcast and notification. */
class SubscriptionAlertsLab : Instrumentation() {
    private lateinit var arguments: Bundle

    override fun onCreate(arguments: Bundle) {
        this.arguments = arguments
        start()
    }

    override fun onStart() {
        val result = Bundle()
        try {
            val schedule = SubscriptionSchedule(targetContext)
            val manager = targetContext.getSystemService(NotificationManager::class.java)
            val ref = "ab3cd900-3fb6-4821-ac19-48cb448f6633"
            val now = System.currentTimeMillis()
            val delay = arguments.getString("delay")?.toLong() ?: 5_000L
            when (arguments.getString("action")) {
                "schedule" -> {
                    schedule.remove(ref)
                    check(schedule.schedule(ref, now + delay, now + delay + 180_000L))
                    check(schedule.schedule(ref, now + delay + 1_000L, now + delay + 180_000L))
                }
                "verify" -> {
                    val notification = manager.activeNotifications.single { it.tag == ref }.notification
                    check(notification.visibility == Notification.VISIBILITY_SECRET)
                    check(notification.extras.getCharSequence(Notification.EXTRA_TITLE).toString() == "확인할 구독 회차가 있습니다")
                    check(notification.extras.getCharSequence(Notification.EXTRA_TEXT) == null)
                    check(notification.contentIntent != null)
                }
                "clear" -> schedule.remove(ref)
                "deny" -> {
                    schedule.remove(ref)
                    check(!manager.areNotificationsEnabled())
                    check(!schedule.schedule(ref, now + delay, now + delay + 180_000L))
                    check(manager.activeNotifications.none { it.tag == ref })
                }
                else -> error("Unknown subscription alerts test action")
            }
            result.putString("subscription_alerts", "pass")
            finish(Activity.RESULT_OK, result)
        } catch (error: Throwable) {
            result.putString("subscription_alerts", "fail")
            result.putString("error", error.stackTraceToString())
            finish(Activity.RESULT_CANCELED, result)
        }
    }
}
