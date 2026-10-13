package com.citopyth.btguard

import android.annotation.SuppressLint
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class Sighting(
    val address: String,
    val name: String?,
    val rssi: Int,
    val smoothed: Double,
    val lastSeen: Long,
)

/**
 * "Detektiv-Radar": scans for nearby devices (Bluetooth LE and classic) and
 * keeps a smoothed signal strength per address, so walking around turns
 * into a warmer/colder game.
 */
class Radar(private val context: Context) {
    private val _seen = MutableStateFlow<Map<String, Sighting>>(emptyMap())
    val seen: StateFlow<Map<String, Sighting>> = _seen.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    private var running = false
    private var receiverRegistered = false

    private val leCallback = object : ScanCallback() {
        override fun onScanResult(callbackType: Int, result: ScanResult) {
            report(result.device, result.scanRecord?.deviceName, result.rssi)
        }

        override fun onBatchScanResults(results: MutableList<ScanResult>) {
            results.forEach { report(it.device, it.scanRecord?.deviceName, it.rssi) }
        }

        override fun onScanFailed(errorCode: Int) {
            _error.value = "BLE-Scan fehlgeschlagen (Code $errorCode)"
        }
    }

    private val classicReceiver = object : BroadcastReceiver() {
        @SuppressLint("MissingPermission")
        override fun onReceive(ctx: Context, intent: Intent) {
            when (intent.action) {
                BluetoothDevice.ACTION_FOUND -> {
                    val device = intent.bluetoothDevice() ?: return
                    val rssi = intent.getShortExtra(BluetoothDevice.EXTRA_RSSI, Short.MIN_VALUE)
                    if (rssi != Short.MIN_VALUE) report(device, null, rssi.toInt())
                }
                // Classic discovery stops after ~12 s; keep it going while the radar runs.
                BluetoothAdapter.ACTION_DISCOVERY_FINISHED -> if (running) {
                    try {
                        Bt.adapter(context)?.startDiscovery()
                    } catch (e: SecurityException) {
                        // permission revoked meanwhile
                    }
                }
            }
        }
    }

    @SuppressLint("MissingPermission")
    fun start() {
        if (running) return
        if (!Bt.canScan(context)) {
            _error.value = "Scan-Berechtigung fehlt"
            return
        }
        val adapter = Bt.adapter(context)
        if (adapter == null || !adapter.isEnabled) {
            _error.value = "Bluetooth ist ausgeschaltet"
            return
        }
        _error.value = null
        running = true
        try {
            adapter.bluetoothLeScanner?.startScan(
                null,
                ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build(),
                leCallback,
            )
            val filter = IntentFilter().apply {
                addAction(BluetoothDevice.ACTION_FOUND)
                addAction(BluetoothAdapter.ACTION_DISCOVERY_FINISHED)
            }
            ContextCompat.registerReceiver(context, classicReceiver, filter, ContextCompat.RECEIVER_NOT_EXPORTED)
            receiverRegistered = true
            adapter.startDiscovery()
        } catch (e: SecurityException) {
            _error.value = "Scan-Berechtigung fehlt"
            stop()
        }
    }

    @SuppressLint("MissingPermission")
    fun stop() {
        running = false
        val adapter = Bt.adapter(context)
        try {
            adapter?.bluetoothLeScanner?.stopScan(leCallback)
            adapter?.cancelDiscovery()
        } catch (e: Exception) {
            // already stopped or permission revoked
        }
        if (receiverRegistered) {
            runCatching { context.unregisterReceiver(classicReceiver) }
            receiverRegistered = false
        }
    }

    private fun report(device: BluetoothDevice, advertisedName: String?, rssi: Int) {
        val address = device.address?.uppercase() ?: return
        val now = System.currentTimeMillis()
        val current = _seen.value
        val prev = current[address]
        // Exponential smoothing keeps the reading calm while walking around.
        val smoothed = if (prev == null || now - prev.lastSeen > 15_000) rssi.toDouble()
        else prev.smoothed * 0.7 + rssi * 0.3
        val name = advertisedName ?: prev?.name ?: Bt.safeName(context, device)
        _seen.value = current + (address to Sighting(address, name, rssi, smoothed, now))
    }

    companion object {
        fun heat(smoothed: Double): Pair<String, String> = when {
            smoothed >= -50 -> "🔥" to "Heiß! Direkt neben dir"
            smoothed >= -65 -> "♨️" to "Warm, ganz in der Nähe"
            smoothed >= -80 -> "🌤️" to "Lauwarm, im selben Raum oder nebenan"
            else -> "❄️" to "Kalt, weiter weg"
        }
    }
}
