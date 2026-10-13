# BT-Wächter

Android-App, die jede Bluetooth-Verbindung **mit deinem Handy** protokolliert,
bei unbekannten oder blockierten Geräten Alarm schlägt und die Einstellungen
prüft, die wirklich vor fremdem Zugriff schützen.

## Funktionen

| Bereich | Was es tut |
|---|---|
| **Geräte** | Name, Adresse, Hersteller (über maclookup.app), zuerst/zuletzt gesehen, Anzahl der Verbindungen. Vertrauen, Blockieren, Orten, Entfernen. Erkennt zweite Adressen bekannter Geräte (z. B. Soundcore: `F4:…` und `CB:…`). |
| **Log** | Jedes Verbinden und Trennen mit Uhrzeit, auch wenn die App geschlossen ist. |
| **Radar** | Detektiv-Modus: Signalstärke als heiß/warm/kalt, um ein Gerät physisch zu finden. |
| **Schutz** | Schutz-Check: gekoppelte Geräte, Sichtbarkeit, USB-/WLAN-Debugging, NFC, Sicherheitsupdate, Akku-Sparmodus. Mit Knopf direkt zur passenden Einstellung. |
| **Spaß** | Tarnname für dein Handy, Köder-Signal unter dem Tarnnamen, Erwischt-Alarm mit Sirene und Sprüchen. |

Beim ersten Start werden alle bereits gekoppelten Geräte als **vertraut**
übernommen (wie eine Gästeliste).

## Grenzen (von Android vorgegeben)

- Android lässt Apps **kein Gerät aussperren** und seit Android 13 **nicht
  entkoppeln**. „Entkoppeln & blockieren“ markiert das Gerät, löst bei jeder
  neuen Verbindung Alarm aus und öffnet die Bluetooth-Einstellungen zum
  Entkoppeln.
- Die App sieht nur Verbindungen **zu deinem Handy**, nicht zwischen fremden
  Geräten und z. B. deiner Musikbox.
- Gegenangriffe auf fremde Geräte gibt es bewusst nicht; sie wären strafbar.

## Installieren

1. Auf GitHub im Repo **Actions → „BT-Wächter APK“** öffnen, den neuesten
   grünen Lauf wählen und unter **Artifacts** `bt-waechter-apk` laden.
2. Die ZIP-Datei entpacken und `bt-waechter.apk` öffnen. Android fragt, ob
   die Installation aus dieser Quelle erlaubt ist: einmal erlauben.
3. Beim ersten Start alle Berechtigungen erteilen.
4. Im Tab **Schutz** bei „Überwachung im Hintergrund“ die App auf
   „Nicht eingeschränkt“ stellen, sonst kann Samsung die Überwachung pausieren.

## Selbst bauen

```bash
cd android-bt-guard
./gradlew assembleDebug   # braucht JDK 17 und das Android SDK
```
