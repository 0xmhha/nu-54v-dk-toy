package com.nu54renter.ble

import android.app.AlarmManager
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.nu54renter.MainActivity

private const val channelId = "nu54_subscription_due"
private const val orderChannelId = "nu54_order_ready"
private val alertId = Regex("^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$")
private const val day = 86_400_000L

// App-private records contain only opaque local references and the lifetime of an OS reminder.
internal class SubscriptionSchedule(private val context: Context) {
    private val registry = context.getSharedPreferences("subscription-reminders", Context.MODE_PRIVATE)
    private val alarms = context.getSystemService(AlarmManager::class.java)
    private val notifications = context.getSystemService(NotificationManager::class.java)

    private fun token(ref: String, flags: Int): PendingIntent? {
        val intent = Intent(context, SubscriptionAlertReceiver::class.java)
            .setData(Uri.parse("nu54://subscriptions/$ref"))
        return PendingIntent.getBroadcast(context, 0, intent, PendingIntent.FLAG_IMMUTABLE or flags)
    }

    private fun timetable(ref: String): Pair<Long, Long>? {
        val dates = registry.getString(ref, null)?.split(':') ?: return null
        val at = dates.getOrNull(0)?.toLongOrNull() ?: return null
        val until = dates.getOrNull(1)?.toLongOrNull() ?: return null
        return at to until
    }

    fun schedule(ref: String, at: Long, until: Long): Boolean {
        val now = System.currentTimeMillis()
        if (!alertId.matches(ref) || at <= now || at >= until || until > now + 91 * day ||
            !notifications.areNotificationsEnabled()) return false
        val existing = timetable(ref)
        // Keep an existing earlier reminder. A new cycle for the same schedule starts after it.
        if (existing != null && existing.second == until && existing.first <= at) return true
        try {
            if (!registry.edit().putString(ref, "$at:$until").commit()) return false
            alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, at, checkNotNull(token(ref, PendingIntent.FLAG_UPDATE_CURRENT)))
            return true
        } catch (_: SecurityException) {
            registry.edit().remove(ref).apply()
            return false
        }
    }

    fun remove(ref: String) {
        if (!alertId.matches(ref)) return
        registry.edit().remove(ref).commit()
        token(ref, PendingIntent.FLAG_NO_CREATE)?.let {
            alarms.cancel(it)
            it.cancel()
        }
        notifications.cancel(ref, 1)
    }

    fun clear() {
        for (ref in registry.all.keys) remove(ref)
    }

    fun restore() {
        val now = System.currentTimeMillis()
        for (ref in registry.all.keys) {
            val (at, until) = timetable(ref) ?: continue
            if (until <= now + 1_000 || !notifications.areNotificationsEnabled()) {
                remove(ref)
            } else if (at > 0) {
                val pending = token(ref, PendingIntent.FLAG_UPDATE_CURRENT) ?: continue
                alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, maxOf(at, now + 1_000), pending)
            }
        }
    }

    fun deliver(ref: String): Boolean {
        val (at, until) = timetable(ref) ?: return false
        val now = System.currentTimeMillis()
        if (at <= 0 || at > now) return false
        if (now >= until || !notifications.areNotificationsEnabled()) {
            remove(ref)
            return false
        }
        // Keep a receipt until this cycle ends so reopening cannot re-arm it.
        return registry.edit().putString(ref, "0:$until").commit()
    }
}

class SubscriptionAlertsModule(private val context: ReactApplicationContext) : NativeSubscriptionAlertsSpec(context) {
    private val schedule = SubscriptionSchedule(context)

    override fun permission(ask: Boolean, promise: Promise) {
        promise.resolve(context.getSystemService(NotificationManager::class.java).areNotificationsEnabled())
    }

    override fun schedule(ref: String, epochMilliseconds: Double, expiresMilliseconds: Double, promise: Promise) {
        promise.resolve(epochMilliseconds.isFinite() && expiresMilliseconds.isFinite() &&
            schedule.schedule(ref, epochMilliseconds.toLong(), expiresMilliseconds.toLong()))
    }

    override fun clear(promise: Promise) {
        schedule.clear()
        promise.resolve(null)
    }

    override fun remove(ref: String, promise: Promise) {
        schedule.remove(ref)
        promise.resolve(null)
    }

    override fun orderReady(order: String, promise: Promise) {
        promise.resolve(showOrderReady(context, order))
    }
}

class SubscriptionAlertReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val schedule = SubscriptionSchedule(context)
        if (intent.action == Intent.ACTION_BOOT_COMPLETED) {
            schedule.restore()
            return
        }
        val ref = intent.data?.lastPathSegment ?: return
        if (!alertId.matches(ref) || !schedule.deliver(ref)) return
        showAlert(context, ref, 1, channelId, "구독 회차", "확인할 구독 회차가 있습니다", "nu54://subscriptions/$ref")
    }
}

internal fun showOrderReady(context: Context, order: String) = alertId.matches(order) &&
    showAlert(context, order, 2, orderChannelId, "주문 준비", "주문한 상품이 준비되었습니다", "nu54://order/ready/$order")

/** Posts a notice that names no order, store or amount; the app shows those after it opens. */
private fun showAlert(context: Context, tag: String, id: Int, channel: String, channelName: String, title: String, route: String): Boolean {
    val manager = context.getSystemService(NotificationManager::class.java)
    if (!manager.areNotificationsEnabled()) return false
    if (Build.VERSION.SDK_INT >= 26) {
        manager.createNotificationChannel(NotificationChannel(channel, channelName, NotificationManager.IMPORTANCE_DEFAULT))
    }
    val target = Intent(Intent.ACTION_VIEW, Uri.parse(route), context, MainActivity::class.java)
    val open = PendingIntent.getActivity(context, 0, target, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
    @Suppress("DEPRECATION")
    val builder = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(context, channel) else Notification.Builder(context)
    val message = builder
        .setSmallIcon(android.R.drawable.ic_dialog_info)
        .setContentTitle(title)
        .setContentIntent(open)
        .setVisibility(Notification.VISIBILITY_SECRET)
        .setAutoCancel(true)
        .build()
    return try {
        manager.notify(tag, id, message)
        true
    } catch (_: SecurityException) {
        false // Permission was revoked after scheduling.
    }
}
