package com.citopyth.btguard

import android.content.Context
import android.content.Intent
import android.nfc.NfcAdapter
import android.os.Build
import android.os.PowerManager
import android.provider.Settings
import java.time.LocalDate
import java.time.temporal.ChronoUnit

data class Check(
    val title: String,
    val ok: Boolean,
    val detail: String,
    val fixLabel: String? = null,
    val fix: ((Context) -> Unit)? = null,
)

/** "Schutz-Check": the settings that actually keep others out of the phone. */
object Defense {
    fun run(context: Context, devices: Map<String, DeviceRecord>): List<Check> = buildList {
        val missing = Bt.missingPermissions(context)
        add(
            Check(
                "Berechtigungen",
                missing.isEmpty(),
                if (missing.isEmpty()) "Alle Berechtigungen erteilt, die Überwachung ist aktiv."
                else "Es fehlen ${missing.size} Berechtigungen. Ohne sie sieht die App keine Verbindungen.",
                "App-Einstellungen".takeIf { missing.isNotEmpty() },
            ) { ctx -> openAppSettings(ctx) },
        )

        // Informational only: Bluetooth being on is normal, not a finding.
        val btOn = Bt.isEnabled(context)
        add(
            Check(
                "Bluetooth",
                true,
                if (btOn) "Bluetooth ist an. Aus ist am sichersten, wenn du es gerade nicht brauchst."
                else "Bluetooth ist aus. Niemand kann sich verbinden.",
                "Bluetooth-Einstellungen",
            ) { ctx -> Bt.openBluetoothSettings(ctx) },
        )

        val discoverable = Bt.isDiscoverable(context)
        add(
            Check(
                "Sichtbarkeit",
                discoverable != true,
                when (discoverable) {
                    true -> "Dein Handy ist gerade für alle sichtbar. Das passiert, solange die Bluetooth-Einstellungen offen sind; schließe sie danach."
                    false -> "Dein Handy ist für fremde Geräte nicht sichtbar."
                    null -> "Unbekannt (Scan-Berechtigung fehlt)."
                },
            ),
        )

        val bonded = Bt.bondedDevices(context)
        val untrusted = bonded.filter { devices[it.first]?.trust != Trust.TRUSTED }
        val blockedBonded = bonded.filter { devices[it.first]?.trust == Trust.BLOCKED }
        add(
            Check(
                "Gekoppelte Geräte",
                untrusted.isEmpty(),
                when {
                    blockedBonded.isNotEmpty() ->
                        "${blockedBonded.size} blockierte Geräte sind noch gekoppelt: " +
                            blockedBonded.joinToString { it.second ?: it.first } + ". Unbedingt entkoppeln!"
                    untrusted.isNotEmpty() ->
                        "${untrusted.size} gekoppelte Geräte sind nicht als vertraut markiert: " +
                            untrusted.joinToString { it.second ?: it.first }
                    else -> "Alle ${bonded.size} gekoppelten Geräte sind als vertraut markiert."
                },
                "Entkoppeln".takeIf { untrusted.isNotEmpty() },
            ) { ctx -> Bt.openBluetoothSettings(ctx) },
        )

        val cr = context.contentResolver
        val adb = Settings.Global.getInt(cr, Settings.Global.ADB_ENABLED, 0) == 1
        add(
            Check(
                "USB-Debugging",
                !adb,
                if (adb) "Ist an. Damit kann ein angeschlossener Computer dein Handy steuern."
                else "Ist aus.",
                "Entwickleroptionen".takeIf { adb },
            ) { ctx -> openDeveloperSettings(ctx) },
        )

        // "adb_wifi_enabled" is the Settings.Global key behind "Debugging über WLAN" (Android 11+).
        val adbWifi = Settings.Global.getInt(cr, "adb_wifi_enabled", 0) == 1
        add(
            Check(
                "Debugging über WLAN",
                !adbWifi,
                if (adbWifi) "Ist an. Geräte im selben WLAN können sich mit deinem Handy koppeln. Ausschalten!"
                else "Ist aus.",
                "Entwickleroptionen".takeIf { adbWifi },
            ) { ctx -> openDeveloperSettings(ctx) },
        )

        val nfc = NfcAdapter.getDefaultAdapter(context)
        if (nfc != null) {
            add(
                Check(
                    "NFC",
                    !nfc.isEnabled,
                    if (nfc.isEnabled) "Ist an. Wenn du nicht mit dem Handy bezahlst, kannst du es ausschalten."
                    else "Ist aus.",
                    "NFC-Einstellungen".takeIf { nfc.isEnabled },
                ) { ctx -> Bt.startFirst(ctx, Intent(Settings.ACTION_NFC_SETTINGS), Intent(Settings.ACTION_SETTINGS)) },
            )
        }

        val patch = runCatching { LocalDate.parse(Build.VERSION.SECURITY_PATCH) }.getOrNull()
        val age = patch?.let { ChronoUnit.DAYS.between(it, LocalDate.now()) }
        add(
            Check(
                "Sicherheitsupdate",
                age != null && age <= 90,
                when {
                    age == null -> "Stand unbekannt."
                    age <= 90 -> "Stand ${Build.VERSION.SECURITY_PATCH}. Aktuell genug."
                    else -> "Stand ${Build.VERSION.SECURITY_PATCH}, also $age Tage alt. Updates schließen bekannte Bluetooth-Lücken."
                },
                "Nach Updates suchen".takeIf { age == null || age > 90 },
            ) { ctx ->
                Bt.startFirst(
                    ctx,
                    Intent("android.settings.SYSTEM_UPDATE_SETTINGS"),
                    Intent(Settings.ACTION_DEVICE_INFO_SETTINGS),
                    Intent(Settings.ACTION_SETTINGS),
                )
            },
        )

        val pm = context.getSystemService(PowerManager::class.java)
        val unrestricted = pm?.isIgnoringBatteryOptimizations(context.packageName) == true
        add(
            Check(
                "Überwachung im Hintergrund",
                unrestricted,
                if (unrestricted) "Der Akku-Sparmodus bremst die App nicht."
                else "Der Akku-Sparmodus kann die Überwachung pausieren. Stelle die App auf „Nicht eingeschränkt“.",
                "Akku-Einstellungen".takeIf { !unrestricted },
            ) { ctx ->
                Bt.startFirst(ctx, Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS), Intent(Settings.ACTION_SETTINGS))
            },
        )
    }

    fun openAppSettings(context: Context) =
        Bt.startFirst(
            context,
            Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, android.net.Uri.fromParts("package", context.packageName, null)),
            Intent(Settings.ACTION_SETTINGS),
        )

    private fun openDeveloperSettings(context: Context) =
        Bt.startFirst(context, Intent(Settings.ACTION_APPLICATION_DEVELOPMENT_SETTINGS), Intent(Settings.ACTION_SETTINGS))
}
