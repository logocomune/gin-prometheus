package com.citopyth.btguard

import android.app.PendingIntent
import android.bluetooth.BluetoothDevice
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import kotlin.concurrent.thread

@Suppress("DEPRECATION")
fun Intent.bluetoothDevice(): BluetoothDevice? =
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        getParcelableExtra(BluetoothDevice.EXTRA_DEVICE, BluetoothDevice::class.java)
    } else {
        getParcelableExtra(BluetoothDevice.EXTRA_DEVICE)
    }

/** Logs every Bluetooth link to this phone, also while the app is closed. */
class AclReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val device = intent.bluetoothDevice() ?: return
        val address = device.address?.uppercase() ?: return
        val name = Bt.safeName(context, device)

        when (intent.action) {
            BluetoothDevice.ACTION_ACL_CONNECTED -> {
                val rec = DeviceStore.recordConnect(context, address, name)
                Alarm.onConnected(context, rec, DeviceStore.state.value.devices.values)
                if (!rec.vendorChecked) {
                    val pending = goAsync()
                    thread {
                        try {
                            Vendor.lookupAndStore(context, address)
                        } finally {
                            pending.finish()
                        }
                    }
                }
            }
            BluetoothDevice.ACTION_ACL_DISCONNECTED -> DeviceStore.recordDisconnect(context, address, name)
        }
    }
}

/** Handles the "Blockieren"/"Vertrauen" buttons in alarm notifications. */
class ActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val address = intent.getStringExtra(EXTRA_ADDRESS) ?: return
        when (intent.action) {
            ACTION_BLOCK -> DeviceStore.setTrust(context, address, Trust.BLOCKED)
            ACTION_TRUST -> DeviceStore.setTrust(context, address, Trust.TRUSTED)
        }
        Alarm.cancel(context, address)
    }

    companion object {
        const val ACTION_BLOCK = "com.citopyth.btguard.BLOCK"
        const val ACTION_TRUST = "com.citopyth.btguard.TRUST"
        private const val EXTRA_ADDRESS = "address"

        fun pending(context: Context, action: String, address: String): PendingIntent =
            PendingIntent.getBroadcast(
                context,
                (action + address).hashCode(),
                Intent(context, ActionReceiver::class.java).setAction(action).putExtra(EXTRA_ADDRESS, address),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
    }
}
