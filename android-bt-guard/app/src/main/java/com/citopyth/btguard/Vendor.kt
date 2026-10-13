package com.citopyth.btguard

import android.content.Context
import java.net.HttpURLConnection
import java.net.URL

/** Resolves manufacturer names via the free maclookup.app API. */
object Vendor {
    private const val NO_COMPANY = "*NO COMPANY*"
    private const val PRIVATE = "*PRIVATE*"

    /** Blocking; call from a background thread. Returns false on network errors. */
    fun lookupAndStore(context: Context, address: String): Boolean {
        if (!MacUtil.mayHaveVendor(address)) {
            DeviceStore.setVendor(context, address, null)
            return true
        }
        val body = fetch(MacUtil.prefix(address)) ?: return false
        val vendor = when (body) {
            NO_COMPANY, "" -> null
            PRIVATE -> "Privat registriert"
            else -> body
        }
        DeviceStore.setVendor(context, address, vendor)
        return true
    }

    private fun fetch(prefix: String): String? {
        val conn = URL("https://api.maclookup.app/v2/macs/$prefix/company/name")
            .openConnection() as HttpURLConnection
        return try {
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.setRequestProperty("User-Agent", "BtGuard/1.0")
            if (conn.responseCode != HttpURLConnection.HTTP_OK) {
                null
            } else {
                conn.inputStream.bufferedReader().use { it.readText().trim() }.take(200)
            }
        } catch (e: Exception) {
            null
        } finally {
            conn.disconnect()
        }
    }
}
