package com.citopyth.btguard

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

enum class Trust { UNKNOWN, TRUSTED, BLOCKED }

enum class EventKind(val label: String) {
    CONNECTED("verbunden"),
    DISCONNECTED("getrennt"),
    BLOCKED_CONNECTED("BLOCKIERTES Gerät verbunden"),
    TRUSTED("als vertraut markiert"),
    BLOCKED("blockiert"),
    UNBLOCKED("Blockierung aufgehoben"),
    REMOVED("entfernt"),
}

data class DeviceRecord(
    val address: String,
    val name: String? = null,
    val vendor: String? = null,
    val vendorChecked: Boolean = false,
    val firstSeen: Long,
    val lastSeen: Long,
    val connectCount: Int = 0,
    val connected: Boolean = false,
    val trust: Trust = Trust.UNKNOWN,
) {
    val label: String get() = name ?: vendor ?: "Unbekanntes Gerät"
}

data class LogEvent(
    val time: Long,
    val address: String,
    val name: String?,
    val kind: EventKind,
)

data class StoreState(
    val devices: Map<String, DeviceRecord> = emptyMap(),
    val events: List<LogEvent> = emptyList(), // newest first
)

/**
 * Persistent device list and event log, shared by the UI and the broadcast
 * receivers (they run in the same process). Stored as one JSON file.
 */
object DeviceStore {
    private const val FILE_NAME = "btguard.json"
    private const val MAX_EVENTS = 500

    private val lock = Any()
    private var file: File? = null
    private val _state = MutableStateFlow(StoreState())
    val state: StateFlow<StoreState> = _state.asStateFlow()

    fun init(context: Context) {
        synchronized(lock) {
            if (file != null) return
            val f = File(context.applicationContext.filesDir, FILE_NAME)
            file = f
            _state.value = runCatching { load(f) }.getOrDefault(StoreState())
        }
    }

    private fun update(context: Context, transform: (StoreState) -> StoreState): StoreState {
        synchronized(lock) {
            init(context)
            val next = transform(_state.value)
            _state.value = next
            runCatching { save(file!!, next) }
            return next
        }
    }

    private fun StoreState.withEvent(e: LogEvent) =
        copy(events = (listOf(e) + events).take(MAX_EVENTS))

    /** Records a connection and returns the updated device. */
    fun recordConnect(context: Context, address: String, name: String?): DeviceRecord {
        val now = System.currentTimeMillis()
        val s = update(context) { s ->
            val old = s.devices[address]
            val rec = (old ?: DeviceRecord(address = address, firstSeen = now, lastSeen = now)).copy(
                name = name ?: old?.name,
                lastSeen = now,
                connectCount = (old?.connectCount ?: 0) + 1,
                connected = true,
            )
            val kind = if (rec.trust == Trust.BLOCKED) EventKind.BLOCKED_CONNECTED else EventKind.CONNECTED
            s.copy(devices = s.devices + (address to rec))
                .withEvent(LogEvent(now, address, rec.name, kind))
        }
        return s.devices.getValue(address)
    }

    fun recordDisconnect(context: Context, address: String, name: String?) {
        val now = System.currentTimeMillis()
        update(context) { s ->
            val old = s.devices[address]
            val rec = (old ?: DeviceRecord(address = address, firstSeen = now, lastSeen = now)).copy(
                name = name ?: old?.name,
                lastSeen = now,
                connected = false,
            )
            s.copy(devices = s.devices + (address to rec))
                .withEvent(LogEvent(now, address, rec.name, EventKind.DISCONNECTED))
        }
    }

    fun setTrust(context: Context, address: String, trust: Trust) {
        val now = System.currentTimeMillis()
        update(context) { s ->
            val old = s.devices[address] ?: DeviceRecord(address = address, firstSeen = now, lastSeen = now)
            if (old.trust == trust) return@update s
            val kind = when (trust) {
                Trust.TRUSTED -> EventKind.TRUSTED
                Trust.BLOCKED -> EventKind.BLOCKED
                Trust.UNKNOWN -> EventKind.UNBLOCKED
            }
            s.copy(devices = s.devices + (address to old.copy(trust = trust)))
                .withEvent(LogEvent(now, address, old.name, kind))
        }
    }

    fun remove(context: Context, address: String) {
        update(context) { s ->
            val old = s.devices[address] ?: return@update s
            s.copy(devices = s.devices - address)
                .withEvent(LogEvent(System.currentTimeMillis(), address, old.name, EventKind.REMOVED))
        }
    }

    fun setVendor(context: Context, address: String, vendor: String?) {
        update(context) { s ->
            val old = s.devices[address] ?: return@update s
            s.copy(devices = s.devices + (address to old.copy(vendor = vendor, vendorChecked = true)))
        }
    }

    /** Adds already-paired devices as trusted; existing entries stay untouched. */
    fun importBonded(context: Context, bonded: List<Pair<String, String?>>) {
        val now = System.currentTimeMillis()
        update(context) { s ->
            val added = bonded.filter { it.first !in s.devices }.associate { (address, name) ->
                address to DeviceRecord(
                    address = address, name = name, firstSeen = now, lastSeen = now, trust = Trust.TRUSTED,
                )
            }
            s.copy(devices = s.devices + added)
        }
    }

    fun clearLog(context: Context) {
        update(context) { it.copy(events = emptyList()) }
    }

    private fun load(f: File): StoreState {
        if (!f.exists()) return StoreState()
        val root = JSONObject(f.readText())
        val devices = root.optJSONArray("devices") ?: JSONArray()
        val events = root.optJSONArray("events") ?: JSONArray()
        return StoreState(
            devices = (0 until devices.length()).map { devices.getJSONObject(it) }.associate { o ->
                val address = o.getString("address")
                address to DeviceRecord(
                    address = address,
                    name = o.optStringOrNull("name"),
                    vendor = o.optStringOrNull("vendor"),
                    vendorChecked = o.optBoolean("vendorChecked"),
                    firstSeen = o.optLong("firstSeen"),
                    lastSeen = o.optLong("lastSeen"),
                    connectCount = o.optInt("connectCount"),
                    connected = o.optBoolean("connected"),
                    trust = runCatching { Trust.valueOf(o.optString("trust")) }.getOrDefault(Trust.UNKNOWN),
                )
            },
            events = (0 until events.length()).mapNotNull { i ->
                val o = events.getJSONObject(i)
                val kind = runCatching { EventKind.valueOf(o.optString("kind")) }.getOrNull() ?: return@mapNotNull null
                LogEvent(o.optLong("time"), o.getString("address"), o.optStringOrNull("name"), kind)
            },
        )
    }

    private fun save(f: File, s: StoreState) {
        val root = JSONObject()
        root.put("devices", JSONArray().apply {
            s.devices.values.forEach { d ->
                put(JSONObject().apply {
                    put("address", d.address)
                    put("name", d.name ?: JSONObject.NULL)
                    put("vendor", d.vendor ?: JSONObject.NULL)
                    put("vendorChecked", d.vendorChecked)
                    put("firstSeen", d.firstSeen)
                    put("lastSeen", d.lastSeen)
                    put("connectCount", d.connectCount)
                    put("connected", d.connected)
                    put("trust", d.trust.name)
                })
            }
        })
        root.put("events", JSONArray().apply {
            s.events.forEach { e ->
                put(JSONObject().apply {
                    put("time", e.time)
                    put("address", e.address)
                    put("name", e.name ?: JSONObject.NULL)
                    put("kind", e.kind.name)
                })
            }
        })
        // Write to a temp file first so a crash never leaves a half-written store.
        val tmp = File(f.parentFile, f.name + ".tmp")
        tmp.writeText(root.toString())
        if (!tmp.renameTo(f)) {
            f.writeText(root.toString())
            tmp.delete()
        }
    }

    private fun JSONObject.optStringOrNull(key: String): String? =
        if (isNull(key)) null else optString(key).takeIf { it.isNotEmpty() }
}
