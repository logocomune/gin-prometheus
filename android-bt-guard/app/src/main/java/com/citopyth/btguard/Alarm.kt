package com.citopyth.btguard

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.provider.Settings
import androidx.core.app.NotificationCompat

/** "Erwischt-Alarm": notifications for unknown and blocked devices. */
object Alarm {
    private const val CHANNEL_SIREN = "siren_v1"
    private const val CHANNEL_QUIET = "quiet_v1"

    private val blockedLines = listOf(
        "%s wollte sich wieder reinschleichen. Die Tür bleibt zu. 🚪",
        "Erwischt! %s steht schon wieder vor der Tür. 🕵️",
        "%s versucht es erneut. Hier wohnt aber nur ein Toaster. 🍞",
        "Alarmstufe Rot: %s ist zurück! 🚨",
    )
    private val unknownLines = listOf(
        "%s klopft an. Kennst du den?",
        "Neues Gesicht: %s. Freund oder Feind?",
        "%s hat sich verbunden. Soll ich die Hunde loslassen? (Spaß, du entscheidest.) 🐕",
        "Psst! %s ist gerade aufgetaucht. 👀",
    )

    fun ensureChannels(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java) ?: return
        val sound = RingtoneManager.getDefaultUri(RingtoneManager.TYPE_ALARM)
            ?: RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION)
        val siren = NotificationChannel(CHANNEL_SIREN, "Erwischt-Alarm (Sirene)", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "Lauter Alarm, wenn sich ein blockiertes oder unbekanntes Gerät verbindet"
            setSound(
                sound,
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_ALARM)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                    .build(),
            )
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 400, 200, 400, 200, 800)
        }
        val quiet = NotificationChannel(CHANNEL_QUIET, "Hinweise", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "Hinweise ohne Sirene"
        }
        nm.createNotificationChannels(listOf(siren, quiet))
    }

    /** Called for every ACL connection. Trusted devices stay silent. */
    fun onConnected(context: Context, rec: DeviceRecord, all: Collection<DeviceRecord>) {
        val twin = all.firstOrNull { it.trust == Trust.TRUSTED && MacUtil.likelySameDevice(it.address, rec.address) }
        when {
            rec.trust == Trust.TRUSTED -> return
            rec.trust == Trust.BLOCKED ->
                notify(context, rec, siren = true, title = "🚨 Erwischt!", text = blockedLines.random().format(rec.label))
            twin != null ->
                notify(
                    context, rec, siren = false,
                    title = "Bekanntes Gerät mit zweiter Adresse?",
                    text = "${rec.address} ähnelt ${twin.label} (${twin.address}). Vermutlich dasselbe Gerät.",
                )
            else ->
                notify(
                    context, rec, siren = Prefs.sirenEnabled(context),
                    title = "Unbekanntes Gerät verbunden",
                    text = unknownLines.random().format(rec.label) + " (${rec.address})",
                )
        }
    }

    fun test(context: Context) {
        val now = System.currentTimeMillis()
        val fake = DeviceRecord(address = "00:00:00:00:00:00", name = "Testalarm", firstSeen = now, lastSeen = now)
        notify(context, fake, siren = Prefs.sirenEnabled(context), title = "🚨 Erwischt! (Test)",
            text = blockedLines.random().format(fake.label), actions = false)
    }

    private fun notify(
        context: Context,
        rec: DeviceRecord,
        siren: Boolean,
        title: String,
        text: String,
        actions: Boolean = true,
    ) {
        ensureChannels(context)
        val id = rec.address.hashCode()
        val open = PendingIntent.getActivity(
            context, id,
            Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val builder = NotificationCompat.Builder(context, if (siren) CHANNEL_SIREN else CHANNEL_QUIET)
            .setSmallIcon(android.R.drawable.stat_sys_warning)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setContentIntent(open)
            .setAutoCancel(true)
            .setPriority(if (siren) NotificationCompat.PRIORITY_HIGH else NotificationCompat.PRIORITY_DEFAULT)
        if (actions) {
            if (rec.trust != Trust.BLOCKED) {
                builder.addAction(0, "Blockieren", ActionReceiver.pending(context, ActionReceiver.ACTION_BLOCK, rec.address))
            }
            builder.addAction(0, "Vertrauen", ActionReceiver.pending(context, ActionReceiver.ACTION_TRUST, rec.address))
            builder.addAction(
                0, "Entkoppeln",
                PendingIntent.getActivity(
                    context, id + 1, Intent(Settings.ACTION_BLUETOOTH_SETTINGS),
                    PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
                ),
            )
        }
        try {
            context.getSystemService(NotificationManager::class.java)?.notify(id, builder.build())
        } catch (e: SecurityException) {
            // Notification permission not granted; the event is still in the log.
        }
    }

    fun cancel(context: Context, address: String) {
        context.getSystemService(NotificationManager::class.java)?.cancel(address.hashCode())
    }
}
