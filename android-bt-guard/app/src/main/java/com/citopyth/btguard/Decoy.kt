package com.citopyth.btguard

import android.annotation.SuppressLint
import android.bluetooth.le.AdvertiseCallback
import android.bluetooth.le.AdvertiseData
import android.bluetooth.le.AdvertiseSettings
import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * "Köder-Gerät": the phone advertises itself under its current (cover)
 * name over Bluetooth LE. The advertisement is not connectable, so nobody
 * can connect through it; it only makes the cover name visible to scanners.
 * It runs as long as the app process lives.
 */
object Decoy {
    private val _running = MutableStateFlow(false)
    val running: StateFlow<Boolean> = _running.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    private val callback = object : AdvertiseCallback() {
        override fun onStartSuccess(settingsInEffect: AdvertiseSettings) {
            _running.value = true
            _error.value = null
        }

        override fun onStartFailure(errorCode: Int) {
            _running.value = false
            _error.value = when (errorCode) {
                AdvertiseCallback.ADVERTISE_FAILED_DATA_TOO_LARGE -> "Name zu lang für das Köder-Signal (max. ca. 26 Zeichen)"
                AdvertiseCallback.ADVERTISE_FAILED_FEATURE_UNSUPPORTED -> "Dein Handy unterstützt kein BLE-Senden"
                AdvertiseCallback.ADVERTISE_FAILED_TOO_MANY_ADVERTISERS -> "Zu viele aktive Sender"
                else -> "Köder konnte nicht starten (Code $errorCode)"
            }
        }
    }

    @SuppressLint("MissingPermission")
    fun start(context: Context) {
        if (_running.value) return
        if (!Bt.canAdvertise(context)) {
            _error.value = "Berechtigung zum Senden fehlt"
            return
        }
        val advertiser = Bt.adapter(context)?.takeIf { it.isEnabled }?.bluetoothLeAdvertiser
        if (advertiser == null) {
            _error.value = "Bluetooth ist aus oder BLE-Senden wird nicht unterstützt"
            return
        }
        try {
            advertiser.startAdvertising(
                AdvertiseSettings.Builder()
                    .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_POWER)
                    .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_MEDIUM)
                    .setConnectable(false)
                    .setTimeout(0)
                    .build(),
                AdvertiseData.Builder().setIncludeDeviceName(true).build(),
                callback,
            )
        } catch (e: SecurityException) {
            _error.value = "Berechtigung zum Senden fehlt"
        }
    }

    @SuppressLint("MissingPermission")
    fun stop(context: Context) {
        try {
            Bt.adapter(context)?.bluetoothLeAdvertiser?.stopAdvertising(callback)
        } catch (e: Exception) {
            // already stopped or permission revoked
        }
        _running.value = false
    }
}
