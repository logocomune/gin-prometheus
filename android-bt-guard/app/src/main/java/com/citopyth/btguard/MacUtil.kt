package com.citopyth.btguard

object MacUtil {
    private val pattern = Regex("^[0-9A-F]{2}(:[0-9A-F]{2}){5}$")

    fun isValid(address: String): Boolean = pattern.matches(address)

    /**
     * True when the address may carry an IEEE-registered manufacturer prefix:
     * neither the multicast bit nor the "locally administered" bit is set.
     * Bluetooth random addresses can still slip through, so a failed lookup
     * is reported as "probably random" rather than as an error.
     */
    fun mayHaveVendor(address: String): Boolean {
        if (!isValid(address)) return false
        val first = address.substring(0, 2).toInt(16)
        return first and 0x03 == 0
    }

    fun prefix(address: String): String = address.replace(":", "").take(6)

    /**
     * Two addresses that differ only in the first byte are very likely the
     * classic and the Bluetooth Low Energy address of the same device (seen,
     * for example, on Soundcore speakers).
     */
    fun likelySameDevice(a: String, b: String): Boolean =
        a != b && isValid(a) && isValid(b) && a.substring(3) == b.substring(3)

    fun kindLabel(address: String): String =
        if (mayHaveVendor(address)) "Herstelleradresse" else "Zufallsadresse"
}
