package com.citopyth.btguard

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import kotlin.concurrent.thread

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        DeviceStore.init(this)
        Alarm.ensureChannels(this)
        setContent {
            MaterialTheme(
                colorScheme = darkColorScheme(
                    primary = Color(0xFF38BDF8),
                    background = Color(0xFF0F172A),
                    surface = Color(0xFF0F172A),
                    surfaceVariant = Color(0xFF1E293B),
                ),
            ) {
                App()
            }
        }
    }
}

private enum class Tab(val icon: String, val label: String) {
    DEVICES("📱", "Geräte"),
    LOG("📜", "Log"),
    RADAR("📡", "Radar"),
    DEFENSE("🛡️", "Schutz"),
    FUN("🎭", "Spaß"),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun App() {
    val context = LocalContext.current
    var tab by rememberSaveable { mutableStateOf(Tab.DEVICES) }
    var radarTarget by rememberSaveable { mutableStateOf<String?>(null) }
    // Bumped after permission changes so screens re-read system state.
    var refresh by remember { mutableIntStateOf(0) }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) {
        importBondedOnce(context)
        refresh++
    }

    LaunchedEffect(Unit) {
        val missing = Bt.missingPermissions(context)
        if (missing.isEmpty()) importBondedOnce(context) else permissionLauncher.launch(missing.toTypedArray())
    }

    Scaffold(
        topBar = { TopAppBar(title = { Text("BT-Wächter · ${tab.label}") }) },
        bottomBar = {
            NavigationBar {
                Tab.entries.forEach { t ->
                    NavigationBarItem(
                        selected = tab == t,
                        onClick = { tab = t },
                        icon = { Text(t.icon) },
                        label = { Text(t.label) },
                    )
                }
            }
        },
    ) { padding ->
        val modifier = Modifier.padding(padding)
        when (tab) {
            Tab.DEVICES -> DevicesScreen(
                modifier,
                onRequestPermissions = { permissionLauncher.launch(Bt.runtimePermissions()) },
                onLocate = { address ->
                    radarTarget = address
                    tab = Tab.RADAR
                },
            )
            Tab.LOG -> LogScreen(modifier)
            Tab.RADAR -> RadarScreen(modifier, radarTarget, onTargetChange = { radarTarget = it })
            Tab.DEFENSE -> DefenseScreen(modifier, refresh)
            Tab.FUN -> FunScreen(modifier)
        }
    }
}

/** On first start, already-paired devices become the trusted "guest list". */
private fun importBondedOnce(context: android.content.Context) {
    if (Prefs.bondedImported(context) || !Bt.canConnect(context)) return
    val bonded = Bt.bondedDevices(context)
    DeviceStore.importBonded(context, bonded)
    Prefs.setBondedImported(context)
    thread { bonded.forEach { Vendor.lookupAndStore(context, it.first) } }
}
