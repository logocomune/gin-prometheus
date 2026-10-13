@file:OptIn(ExperimentalMaterial3Api::class)

package com.citopyth.btguard

import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.concurrent.thread
import kotlin.math.roundToInt

private val timeFormat = DateTimeFormatter.ofPattern("dd.MM.yy HH:mm:ss").withZone(ZoneId.systemDefault())

private fun fmt(ms: Long): String = if (ms <= 0) "–" else timeFormat.format(Instant.ofEpochMilli(ms))

private fun toast(context: Context, msg: String) = Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

private val Red = Color(0xFFF87171)
private val Green = Color(0xFF4ADE80)
private val Amber = Color(0xFFFBBF24)

private fun trustIcon(t: Trust) = when (t) {
    Trust.TRUSTED -> "✅"
    Trust.BLOCKED -> "⛔"
    Trust.UNKNOWN -> "❓"
}

// ---------------------------------------------------------------------------
// Geräte
// ---------------------------------------------------------------------------

private enum class Filter(val label: String) { ALL("Alle"), UNKNOWN("Unbekannt"), BLOCKED("Blockiert"), TRUSTED("Vertraut") }

private fun Filter.matches(d: DeviceRecord) = when (this) {
    Filter.ALL -> true
    Filter.UNKNOWN -> d.trust == Trust.UNKNOWN
    Filter.BLOCKED -> d.trust == Trust.BLOCKED
    Filter.TRUSTED -> d.trust == Trust.TRUSTED
}

@Composable
fun DevicesScreen(modifier: Modifier, onRequestPermissions: () -> Unit, onLocate: (String) -> Unit) {
    val context = LocalContext.current
    val state by DeviceStore.state.collectAsState()
    var filter by rememberSaveable { mutableStateOf(Filter.ALL) }
    var toRemove by remember { mutableStateOf<DeviceRecord?>(null) }
    val all = state.devices.values
    val devices = all.filter { filter.matches(it) }
        .sortedWith(compareByDescending<DeviceRecord> { it.connected }.thenByDescending { it.lastSeen })

    LazyColumn(
        modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (Bt.missingPermissions(context).isNotEmpty()) {
            item {
                Card(colors = CardDefaults.cardColors(containerColor = Color(0xFF422006))) {
                    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("Berechtigungen fehlen", style = MaterialTheme.typography.titleMedium)
                        Text("Ohne sie kann die App keine Verbindungen sehen. Wurden sie zweimal abgelehnt, erteile sie in den App-Einstellungen.")
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(onClick = onRequestPermissions) { Text("Erlauben") }
                            OutlinedButton(onClick = { Defense.openAppSettings(context) }) { Text("App-Einstellungen") }
                        }
                    }
                }
            }
        }
        item {
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Filter.entries.forEach { f ->
                    FilterChip(
                        selected = filter == f,
                        onClick = { filter = f },
                        label = { Text("${f.label} (${all.count { f.matches(it) }})") },
                    )
                }
            }
        }
        item {
            OutlinedButton(onClick = {
                val open = all.filter { !it.vendorChecked }.map { it.address }
                if (open.isEmpty()) {
                    toast(context, "Alle Hersteller sind bereits nachgeschlagen.")
                } else {
                    toast(context, "Schlage ${open.size} Hersteller nach …")
                    thread { open.forEach { Vendor.lookupAndStore(context.applicationContext, it) } }
                }
            }) { Text("Hersteller nachschlagen") }
        }
        if (devices.isEmpty()) {
            item {
                Text(
                    "Noch keine Geräte. Sobald sich etwas per Bluetooth mit deinem Handy verbindet, taucht es hier auf.",
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        items(devices, key = { it.address }) { d ->
            val twin = all.firstOrNull { it.address != d.address && MacUtil.likelySameDevice(it.address, d.address) }
            DeviceCard(
                d, twin,
                onTrust = { DeviceStore.setTrust(context, d.address, Trust.TRUSTED) },
                onBlockToggle = {
                    DeviceStore.setTrust(context, d.address, if (d.trust == Trust.BLOCKED) Trust.UNKNOWN else Trust.BLOCKED)
                },
                onLocate = { onLocate(d.address) },
                onRemove = { toRemove = d },
            )
        }
    }

    toRemove?.let { d ->
        AlertDialog(
            onDismissRequest = { toRemove = null },
            title = { Text("${d.label} entfernen?") },
            text = {
                Text(
                    "„Entkoppeln & blockieren“ trennt die Kopplung und löst künftig sofort Alarm aus, " +
                        "wenn sich das Gerät wieder verbindet.\n\n„Nur löschen“ vergisst das Gerät in dieser App.",
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    removeAndBlock(context, d)
                    toRemove = null
                }) { Text("Entkoppeln & blockieren", color = Red) }
            },
            dismissButton = {
                Row {
                    TextButton(onClick = {
                        DeviceStore.remove(context, d.address)
                        toRemove = null
                    }) { Text("Nur löschen") }
                    TextButton(onClick = { toRemove = null }) { Text("Abbrechen") }
                }
            },
        )
    }
}

private fun removeAndBlock(context: Context, d: DeviceRecord) {
    DeviceStore.setTrust(context, d.address, Trust.BLOCKED)
    if (Bt.isBonded(context, d.address) && !Bt.tryUnpair(context, d.address)) {
        toast(
            context,
            "Blockiert. Android erlaubt das Entkoppeln nur in den Einstellungen: " +
                "Zahnrad neben „${d.label}“ → Entkoppeln.",
        )
        Bt.openBluetoothSettings(context)
    } else {
        toast(context, "Blockiert. Verbindet es sich wieder, gibt es sofort Alarm.")
    }
}

@Composable
private fun DeviceCard(
    d: DeviceRecord,
    twin: DeviceRecord?,
    onTrust: () -> Unit,
    onBlockToggle: () -> Unit,
    onLocate: () -> Unit,
    onRemove: () -> Unit,
) {
    val container = when (d.trust) {
        Trust.BLOCKED -> Color(0xFF450A0A)
        Trust.UNKNOWN -> Color(0xFF3B2A06)
        Trust.TRUSTED -> MaterialTheme.colorScheme.surfaceVariant
    }
    val vendorText = when {
        d.vendor != null -> d.vendor
        !d.vendorChecked -> "Hersteller noch nicht nachgeschlagen"
        else -> "kein Hersteller (vermutlich Zufallsadresse)"
    }
    Card(Modifier.fillMaxWidth(), colors = CardDefaults.cardColors(containerColor = container)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("${trustIcon(d.trust)} ${d.label}", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                if (d.connected) Text("● verbunden", color = Green, style = MaterialTheme.typography.labelMedium)
            }
            Text("${d.address} · ${MacUtil.kindLabel(d.address)}", style = MaterialTheme.typography.bodySmall)
            Text(vendorText, style = MaterialTheme.typography.bodySmall)
            Text("Zuerst gesehen: ${fmt(d.firstSeen)}", style = MaterialTheme.typography.bodySmall)
            Text("Zuletzt gesehen: ${fmt(d.lastSeen)}", style = MaterialTheme.typography.bodySmall)
            Text("Verbindungen: ${d.connectCount}", style = MaterialTheme.typography.bodySmall)
            if (twin != null) {
                Text(
                    "🔗 Vermutlich zweite Adresse von ${twin.label} (${twin.address})",
                    color = MaterialTheme.colorScheme.primary,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Row(Modifier.horizontalScroll(rememberScrollState())) {
                if (d.trust != Trust.TRUSTED) TextButton(onClick = onTrust) { Text("Vertrauen") }
                TextButton(onClick = onBlockToggle) { Text(if (d.trust == Trust.BLOCKED) "Freigeben" else "Blockieren") }
                TextButton(onClick = onLocate) { Text("Orten") }
                TextButton(onClick = onRemove) { Text("Entfernen", color = Red) }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Log
// ---------------------------------------------------------------------------

@Composable
fun LogScreen(modifier: Modifier) {
    val context = LocalContext.current
    val state by DeviceStore.state.collectAsState()
    var confirmClear by remember { mutableStateOf(false) }

    LazyColumn(
        modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("${state.events.size} Ereignisse", modifier = Modifier.weight(1f))
                if (state.events.isNotEmpty()) TextButton(onClick = { confirmClear = true }) { Text("Log leeren") }
            }
        }
        items(state.events) { e ->
            val color = when (e.kind) {
                EventKind.BLOCKED_CONNECTED -> Red
                EventKind.CONNECTED -> if (state.devices[e.address]?.trust == Trust.TRUSTED) Green else Amber
                else -> MaterialTheme.colorScheme.onSurface
            }
            Column {
                Text(fmt(e.time), style = MaterialTheme.typography.labelSmall)
                Text("${e.name ?: "Unbekannt"} · ${e.address}", style = MaterialTheme.typography.bodyMedium)
                Text(e.kind.label, color = color, style = MaterialTheme.typography.bodySmall)
            }
        }
    }

    if (confirmClear) {
        AlertDialog(
            onDismissRequest = { confirmClear = false },
            title = { Text("Log leeren?") },
            text = { Text("Die Geräteliste bleibt erhalten, nur die Ereignisse werden gelöscht.") },
            confirmButton = {
                TextButton(onClick = {
                    DeviceStore.clearLog(context)
                    confirmClear = false
                }) { Text("Leeren") }
            },
            dismissButton = { TextButton(onClick = { confirmClear = false }) { Text("Abbrechen") } },
        )
    }
}

// ---------------------------------------------------------------------------
// Detektiv-Radar
// ---------------------------------------------------------------------------

@Composable
fun RadarScreen(modifier: Modifier, target: String?, onTargetChange: (String?) -> Unit) {
    val context = LocalContext.current
    val radar = remember { Radar(context.applicationContext) }
    DisposableEffect(Unit) {
        radar.start()
        onDispose { radar.stop() }
    }
    val seen by radar.seen.collectAsState()
    val error by radar.error.collectAsState()
    val store by DeviceStore.state.collectAsState()
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(1000)
            now = System.currentTimeMillis()
        }
    }
    val nearby = seen.values.filter { now - it.lastSeen < 30_000 }.sortedByDescending { it.smoothed }

    LazyColumn(
        modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        error?.let { msg ->
            item {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(msg, color = Red)
                    Button(onClick = {
                        radar.stop()
                        radar.start()
                    }) { Text("Neu starten") }
                }
            }
        }
        if (target != null) {
            item {
                TargetPanel(target, seen[target], store.devices[target], now, onClear = { onTargetChange(null) })
            }
        }
        item {
            Text("In der Nähe (${nearby.size})", style = MaterialTheme.typography.titleMedium)
            Text(
                "Tippe auf ein Gerät, um es zu orten. Handys wechseln ihre BLE-Adresse alle paar Minuten. " +
                    "Geräte, die gerade mit einem anderen Gerät verbunden sind, senden oft nichts.",
                style = MaterialTheme.typography.bodySmall,
            )
        }
        items(nearby, key = { it.address }) { s ->
            val rec = store.devices[s.address]
            val (emoji, _) = Radar.heat(s.smoothed)
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickable { onTargetChange(s.address) }
                    .padding(vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(emoji, fontSize = 22.sp)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        (rec?.let { trustIcon(it.trust) + " " } ?: "") + (s.name ?: rec?.label ?: "Unbekannt"),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    Text(s.address, style = MaterialTheme.typography.bodySmall)
                }
                Text("${s.smoothed.roundToInt()} dBm", style = MaterialTheme.typography.labelMedium)
            }
        }
    }
}

@Composable
private fun TargetPanel(address: String, s: Sighting?, rec: DeviceRecord?, now: Long, onClear: () -> Unit) {
    Card(Modifier.fillMaxWidth()) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text("Ziel: ${rec?.label ?: s?.name ?: "Unbekannt"}", style = MaterialTheme.typography.titleMedium)
            Text(address, style = MaterialTheme.typography.bodySmall)
            if (s == null || now - s.lastSeen > 10_000) {
                Text("🔍", fontSize = 56.sp)
                Text("Nicht in Reichweite oder sendet gerade nicht. Geh ein paar Schritte.")
            } else {
                val (emoji, label) = Radar.heat(s.smoothed)
                Text(emoji, fontSize = 64.sp)
                Text(label, style = MaterialTheme.typography.titleLarge)
                Text("Signal: ${s.smoothed.roundToInt()} dBm (roh ${s.rssi})")
                LinearProgressIndicator(
                    progress = { ((s.smoothed + 100) / 70).toFloat().coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            TextButton(onClick = onClear) { Text("Ziel aufheben") }
        }
    }
}

// ---------------------------------------------------------------------------
// Schutz-Check
// ---------------------------------------------------------------------------

@Composable
fun DefenseScreen(modifier: Modifier, refresh: Int) {
    val context = LocalContext.current
    val store by DeviceStore.state.collectAsState()
    var tick by remember { mutableIntStateOf(0) }
    val checks = remember(refresh, tick, store.devices) { Defense.run(context, store.devices) }
    val okCount = checks.count { it.ok }

    LazyColumn(
        modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    "$okCount von ${checks.size} Punkten in Ordnung",
                    style = MaterialTheme.typography.headlineSmall,
                    color = if (okCount == checks.size) Green else Amber,
                )
                Text(
                    "Das ist die echte Verteidigung: Diese Einstellungen entscheiden, wer an dein Handy kommt. " +
                        "Nach einer Änderung zurückkommen und „Neu prüfen“ tippen.",
                    style = MaterialTheme.typography.bodySmall,
                )
                OutlinedButton(onClick = { tick++ }) { Text("Neu prüfen") }
            }
        }
        items(checks) { c ->
            Card(Modifier.fillMaxWidth()) {
                Row(Modifier.padding(12.dp)) {
                    Text(if (c.ok) "✅" else "⚠️", fontSize = 22.sp)
                    Spacer(Modifier.width(10.dp))
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(c.title, style = MaterialTheme.typography.titleMedium)
                        Text(c.detail, style = MaterialTheme.typography.bodyMedium)
                        val fix = c.fix
                        if (c.fixLabel != null && fix != null) {
                            Button(onClick = { fix(context) }) { Text(c.fixLabel) }
                        }
                    }
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Spaß: Tarnname, Köder, Erwischt-Alarm
// ---------------------------------------------------------------------------

private val coverNames = listOf(
    "Toaster 3000",
    "Omas Hörgerät",
    "Kaffeemaschine",
    "Nichts zu sehen hier",
    "404 Gerät nicht gefunden",
    "Bitte nicht stören",
)

@Composable
fun FunScreen(modifier: Modifier) {
    val context = LocalContext.current
    var currentName by remember { mutableStateOf(Bt.adapterName(context)) }
    var originalName by remember { mutableStateOf(Prefs.originalName(context)) }
    var input by rememberSaveable { mutableStateOf("") }
    val decoyRunning by Decoy.running.collectAsState()
    val decoyError by Decoy.error.collectAsState()
    var siren by remember { mutableStateOf(Prefs.sirenEnabled(context)) }

    fun applyName(name: String) {
        if (originalName == null) {
            currentName?.let {
                Prefs.setOriginalName(context, it)
                originalName = it
            }
        }
        if (Bt.setAdapterName(context, name)) {
            currentName = name
            toast(context, "Dein Handy heißt für Bluetooth jetzt „$name“.")
            // The decoy advertises the name it started with; restart it with the new one.
            if (decoyRunning) {
                Decoy.stop(context)
                Decoy.start(context)
            }
        } else {
            toast(context, "Umbenennen nicht möglich. Ist Bluetooth an und die Berechtigung erteilt?")
        }
    }

    Column(
        modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text("🥸 Tarnname", style = MaterialTheme.typography.titleLarge)
        Text("Aktueller Bluetooth-Name: ${currentName ?: "unbekannt"}")
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            coverNames.forEach { n -> OutlinedButton(onClick = { input = n }) { Text(n) } }
        }
        OutlinedTextField(
            value = input,
            onValueChange = { input = it.take(26) },
            label = { Text("Tarnname") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(enabled = input.isNotBlank(), onClick = { applyName(input.trim()) }) { Text("Tarnen") }
            OutlinedButton(
                enabled = originalName != null,
                onClick = {
                    originalName?.let { original ->
                        if (Bt.setAdapterName(context, original)) {
                            Prefs.clearOriginalName(context)
                            originalName = null
                            currentName = original
                            toast(context, "Echter Name „$original“ ist zurück.")
                        }
                    }
                },
            ) { Text("Echten Namen zurück") }
        }
        Text(
            "Den Namen sehen alle, die nach Bluetooth-Geräten suchen, und deine gekoppelten Geräte.",
            style = MaterialTheme.typography.bodySmall,
        )

        Spacer(Modifier.padding(4.dp))
        Text("🎣 Köder-Gerät", style = MaterialTheme.typography.titleLarge)
        Text(
            "Dein Handy sendet ein Signal unter seinem aktuellen Namen (${currentName ?: "unbekannt"}). " +
                "Wer scannt, sieht nur den Köder. Verbinden kann sich darüber niemand.",
        )
        Row(verticalAlignment = Alignment.CenterVertically) {
            Switch(checked = decoyRunning, onCheckedChange = { on -> if (on) Decoy.start(context) else Decoy.stop(context) })
            Spacer(Modifier.width(10.dp))
            Text(if (decoyRunning) "Köder aktiv" else "Köder aus")
        }
        decoyError?.let { Text(it, color = Red) }
        Text("Läuft, solange die App im Hintergrund weiterlebt.", style = MaterialTheme.typography.bodySmall)

        Spacer(Modifier.padding(4.dp))
        Text("🚨 Erwischt-Alarm", style = MaterialTheme.typography.titleLarge)
        Row(verticalAlignment = Alignment.CenterVertically) {
            Switch(checked = siren, onCheckedChange = {
                siren = it
                Prefs.setSirenEnabled(context, it)
            })
            Spacer(Modifier.width(10.dp))
            Text("Sirene bei unbekannten Geräten")
        }
        Text("Bei blockierten Geräten heult die Sirene immer.", style = MaterialTheme.typography.bodySmall)
        OutlinedButton(onClick = { Alarm.test(context) }) { Text("Testalarm auslösen") }
    }
}
