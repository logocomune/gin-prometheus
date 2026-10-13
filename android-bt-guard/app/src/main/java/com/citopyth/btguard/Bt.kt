package com.citopyth.btguard

import android.Manifest
import android.annotation.SuppressLint
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.provider.Settings
import androidx.core.content.ContextCompat

/** Bluetooth helpers. Every call degrades gracefully when a permission is missing. */
object Bt {
    fun adapter(context: Context): BluetoothAdapter? =
        context.getSystemService(BluetoothManager::class.java)?.adapter

    fun runtimePermissions(): Array<String> = buildList {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            add(Manifest.permission.BLUETOOTH_CONNECT)
            add(Manifest.permission.BLUETOOTH_SCAN)
            add(Manifest.permission.BLUETOOTH_ADVERTISE)
        } else {
            add(Manifest.permission.ACCESS_FINE_LOCATION)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            add(Manifest.permission.POST_NOTIFICATIONS)
        }
    }.toTypedArray()

    fun missingPermissions(context: Context): List<String> =
        runtimePermissions().filter {
            ContextCompat.checkSelfPermission(context, it) != PackageManager.PERMISSION_GRANTED
        }

    private fun has(context: Context, permission: String) =
        ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED

    fun canConnect(context: Context) =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.S || has(context, Manifest.permission.BLUETOOTH_CONNECT)

    fun canScan(context: Context) =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) has(context, Manifest.permission.BLUETOOTH_SCAN)
        else has(context, Manifest.permission.ACCESS_FINE_LOCATION)

    fun canAdvertise(context: Context) =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.S || has(context, Manifest.permission.BLUETOOTH_ADVERTISE)

    @SuppressLint("MissingPermission")
    fun safeName(context: Context, device: BluetoothDevice): String? {
        if (!canConnect(context)) return null
        return try {
            device.name?.takeIf { it.isNotBlank() }
        } catch (e: SecurityException) {
            null
        }
    }

    @SuppressLint("MissingPermission")
    fun bondedDevices(context: Context): List<Pair<String, String?>> {
        if (!canConnect(context)) return emptyList()
        return try {
            adapter(context)?.bondedDevices.orEmpty().map { it.address.uppercase() to safeName(context, it) }
        } catch (e: SecurityException) {
            emptyList()
        }
    }

    fun isBonded(context: Context, address: String): Boolean =
        bondedDevices(context).any { it.first == address }

    /**
     * Tries to unpair a device. Android has no public API for this, so the
     * hidden removeBond() is called via reflection; recent Android versions
     * refuse it, in which case the caller sends the user to the system
     * Bluetooth settings. Returns true if the device is (now) not bonded.
     */
    @SuppressLint("MissingPermission")
    fun tryUnpair(context: Context, address: String): Boolean {
        if (!MacUtil.isValid(address) || !canConnect(context)) return false
        return try {
            val device = adapter(context)?.getRemoteDevice(address) ?: return false
            if (device.bondState == BluetoothDevice.BOND_NONE) return true
            device.javaClass.getMethod("removeBond").invoke(device) as? Boolean ?: false
        } catch (e: Exception) {
            false
        }
    }

    @SuppressLint("MissingPermission")
    fun adapterName(context: Context): String? {
        if (!canConnect(context)) return null
        return try {
            adapter(context)?.name
        } catch (e: SecurityException) {
            null
        }
    }

    @SuppressLint("MissingPermission")
    fun setAdapterName(context: Context, name: String): Boolean {
        if (!canConnect(context)) return false
        return try {
            adapter(context)?.setName(name) ?: false
        } catch (e: SecurityException) {
            false
        }
    }

    @SuppressLint("MissingPermission")
    fun isDiscoverable(context: Context): Boolean? {
        if (!canScan(context)) return null
        return try {
            adapter(context)?.scanMode == BluetoothAdapter.SCAN_MODE_CONNECTABLE_DISCOVERABLE
        } catch (e: SecurityException) {
            null
        }
    }

    fun isEnabled(context: Context): Boolean = adapter(context)?.isEnabled == true

    fun openBluetoothSettings(context: Context) =
        startFirst(context, Intent(Settings.ACTION_BLUETOOTH_SETTINGS), Intent(Settings.ACTION_SETTINGS))

    /** Starts the first intent the device can handle. */
    fun startFirst(context: Context, vararg intents: Intent): Boolean {
        for (intent in intents) {
            try {
                context.startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                return true
            } catch (e: ActivityNotFoundException) {
                // try the next one
            } catch (e: SecurityException) {
                // try the next one
            }
        }
        return false
    }
}
