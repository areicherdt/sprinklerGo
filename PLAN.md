# Port-Plan: sprinklers_pi → sprinklerGo

Analyse der bestehenden C++-Anwendung `sprinklers_pi-master` und Plan für einen Port
nach **Go** mit REST-API und modernem Web-Interface. Scope: **nur Basisfunktionen** (Phase 1).

> **Umsetzungsstand (2026-07-07):** Alle Meilensteine M1–M6 abgeschlossen und
> getestet — Modelle/Stores, Engine mit Semantik-Paritätstests, Hardware-Backends,
> REST-API, React-Frontend im Binary, Open-Meteo-Provider (live verifiziert),
> systemd-Unit und Deployment-Doku. Siehe README.md.
>
> **Phase 2:** Der Modernisierungsplan (Workflow, UI, Prozesse; M7–M12) steht in
> [PLAN-MODERNISIERUNG.md](PLAN-MODERNISIERUNG.md).

---

## 1. Analyse der bestehenden Anwendung

### 1.1 Überblick

`sprinklers_pi` (Richard Zimmerman, ~5.000 Zeilen C++) ist eine Bewässerungssteuerung
für den Raspberry Pi (und Arduino/AVR). Ein einziger Prozess enthält:

- einen **Single-Threaded-Hauptloop** (1-ms-Tick) in [core.cpp](sprinklers_pi-master/core.cpp),
- einen **eingebauten HTTP-Server** ([web.cpp](sprinklers_pi-master/web.cpp), selbstgeschriebener Header-Parser),
- **EEPROM-emulierte Persistenz** (binäres Speicherabbild mit festen Adress-Offsets, [settings.h](sprinklers_pi-master/settings.h)),
- **SQLite-Logging** der Bewässerungsereignisse ([Logging.cpp](sprinklers_pi-master/Logging.cpp)),
- **Wetter-Provider** (zur Compile-Zeit gewählt: OpenMeteo, OpenWeather, Aeris, DarkSky†, Wunderground†),
- **Hardware-Ansteuerung** über wiringPi (GPIO direkt, OpenSprinkler-Schieberegister oder externes Skript),
- ein **jQuery-Mobile-Frontend** (statische Seiten unter `web/`).

† DarkSky und Wunderground sind tot (APIs abgeschaltet).

### 1.2 Datenmodell

**Zone** (max. 15, fest): `name` (20 Zeichen), `enabled`, `pump` (Master-Ventil/Pumpe mitschalten).
Ausgang 0 ist das Pumpen-/Masterventil, Zonen belegen Ausgänge 1–15.

**Schedule** (max. 10): `name`, `enabled`, Typ **Wochentage** (Bitmaske So–Sa) **oder**
**Intervall** (alle N Tage, berechnet über `elapsedDays % interval`), optionale
**Gerade/Ungerade-Restriktion** (Tag des Monats), Flag **Wetteranpassung**,
bis zu **4 Startzeiten** (Minuten seit Mitternacht, -1 = aus),
**Dauer pro Zone** in Minuten (0–255).

**Settings**: Web-Port, Output-Typ (`none`/`direct+`/`direct-`/`OpenSprinkler`),
saisonale Anpassung in % (0–200), Wetter-Provider-Zugangsdaten (API-Key/Secret, Ort),
"Run Schedules" (globaler Ein/Aus-Schalter), Netzwerk/NTP (nur Arduino-relevant).

**Log**: SQLite-Tabelle `zonelog(date, zone, duration, schedule, seasonal, wunderground)` —
ein Eintrag pro gelaufener Zone inkl. angewandter Anpassungsfaktoren.

### 1.3 Scheduling-Semantik (muss der Port exakt erhalten)

1. Um Mitternacht (und nach jeder Änderung) wird eine **Tages-Eventliste** aufgebaut:
   für jeden heute fälligen Schedule und jede Startzeit ein "Schedule starten"-Event.
2. Beim Start eines Schedules werden die Zonen-Events **sequentiell** expandiert:
   Zone für Zone nacheinander (nie parallel), jeweils mit angepasster Dauer.
3. **Dauer-Anpassung**: `dauer × seasonal% × wetter% / 10000`, gedeckelt auf 255 min.
   Wetter-Skalierung (0–200 %) aus Vortageswerten: `100 + humid_factor + temp_factor + rain_factor`
   mit `humid = 30 − Ø-Luftfeuchte`, `temp = (Ø-Temp°F − 70) × 4`, `rain = (Regen gestern + heute) × −2`.
4. Läuft bereits ein Schedule, wird ein neu fälliger Start **minutenweise verschoben**.
5. **Manuellbetrieb**: einzelne Zone an/aus, ohne Endzeit (bis manuell gestoppt).
   Pumpe wird automatisch mitgeschaltet, wenn die Zone `pump=true` hat.
6. **Quick Schedule**: Ad-hoc-Lauf, entweder ein bestehender Schedule sofort oder
   frei eingegebene Zonen-Dauern.
7. Bekannte Lücke im Original (Code-TODO): Schedules, die über Mitternacht laufen.

### 1.4 HTTP-Schnittstelle des Originals

Alles läuft über **GET mit Query-Parametern** (auch schreibende Operationen!),
Antworten sind handformatiertes JSON mit `Content-Type: text/plain`:

| Alt (GET)          | Funktion                                            |
|--------------------|-----------------------------------------------------|
| `json/state`       | Systemstatus (Version, aktive Zone, Restzeit, …)    |
| `json/zones`       | Zonenliste inkl. Live-Zustand                       |
| `json/schedules`   | Schedule-Liste inkl. "next run"                     |
| `json/schedule?id=`| Einzelner Schedule (voll)                           |
| `json/settings`    | Einstellungen                                       |
| `json/wcheck`      | Wetter-Provider-Diagnose                            |
| `json/logs`        | Log-Daten für Graphen (Gruppierung h/d/m)           |
| `json/tlogs`       | Log-Daten als Tabelle                               |
| `bin/setZones`     | Zonen speichern                                     |
| `bin/setSched`     | Schedule anlegen/ändern (`id=-1` = neu)             |
| `bin/delSched`     | Schedule löschen                                    |
| `bin/setQSched`    | Quick Schedule starten                              |
| `bin/manual`       | Zone manuell an/aus                                 |
| `bin/run`          | Scheduler global an/aus                             |
| `bin/settings`     | Einstellungen speichern                             |
| `bin/chatter`      | Relais-Klick-Test (ChatterBox)                      |
| `bin/factory`      | Werksreset                                          |
| `bin/reset`        | Prozess-Neustart                                    |

### 1.5 Hardware-Abstraktion

`io_latch()` schreibt ein 16-Bit-Zustandswort auf einen von vier Backends:

- **OT_NONE** → externes Skript `/usr/local/bin/sprinklers_pi_zone <zone> <0|1>` pro Ausgang
- **OT_DIRECT_POS / OT_DIRECT_NEG** → GPIO direkt (wiringPi), aktiv-high/-low
- **OT_OPEN_SPRINKLER** → Schieberegister (CLK/DAT/LAT/NOE-Pins)

---

## 2. Zielarchitektur des Ports

### 2.1 Technologie-Entscheidungen

| Baustein     | Wahl                                   | Begründung |
|--------------|----------------------------------------|------------|
| Sprache      | **Go** (≥ 1.22)                        | Projektname `sprinklerGo`; ein statisches Binary, trivialer Cross-Compile für Raspberry Pi (`GOOS=linux GOARCH=arm64`), eingebauter HTTP-Server, Goroutinen ersetzen den 1-ms-Loop sauber |
| HTTP/API     | `net/http` Stdlib (Method-Routing des 1.22-ServeMux) | Keine Framework-Abhängigkeit nötig; REST + JSON |
| Frontend     | **React + TypeScript + Vite**, Build via `go:embed` ins Binary | Modern, mobil-tauglich; Deployment bleibt "ein Binary" wie im Original |
| Konfiguration| **eine JSON-Datei** (`config.json`: Settings + Zonen + Schedules), atomar geschrieben | Ersetzt das EEPROM-Blob; menschenlesbar, versionierbar, kein Migrationstool-Zwang |
| Log-DB       | **SQLite** via `modernc.org/sqlite` (CGO-frei)      | Gleiches Schema-Konzept wie Original, Cross-Compile bleibt trivial |
| GPIO         | `periph.io` oder `warthog618/go-gpiocdev` (Linux gpiochip) | wiringPi ist deprecated; gpiochip-API ist der heutige Standard |
| Wetter       | Interface + **Open-Meteo** als erster Provider      | Einziger Provider ohne API-Key; Interface hält OpenWeather für Phase 2 offen |
| Zeit         | Systemzeit (`time.Time`, lokale TZ)                 | NTP-Handling des Originals ist nur für Arduino relevant |
| Deployment   | systemd-Unit statt init.d                           | Zeitgemäß auf Raspberry Pi OS |

### 2.2 Projektstruktur

```
sprinklerGo/
├── cmd/sprinklerd/main.go        # Entry Point, Flags (-config, -port), Signal-Handling
├── internal/
│   ├── model/                    # Zone, Schedule, Settings (+ JSON-Tags, Validierung)
│   ├── store/                    # config.json laden/atomar speichern; SQLite-Logstore
│   ├── engine/                   # Scheduler: Eventliste, Tick-Loop, RunState
│   ├── hardware/                 # Interface Output + Backends: mock, script, gpio
│   ├── weather/                  # Interface Provider + dummy, openmeteo
│   └── api/                      # HTTP-Handler, Routing, statisches Frontend (embed)
├── web/                          # React/TS/Vite-Frontend
│   └── src/...
├── deploy/sprinklerd.service     # systemd-Unit
└── PLAN.md
```

### 2.3 Kernkomponenten

**Engine** (ersetzt `core.cpp`): eine Goroutine mit 1-Sekunden-Ticker.
Zustand (Eventliste, RunState) hinter einem Mutex; die API ruft Methoden wie
`engine.StartQuickRun()`, `engine.SetManual()`, `engine.Reload()` auf.
Die Semantik aus §1.3 wird 1:1 übernommen (inkl. Minuten-Verschiebung bei Kollision
und Mitternachts-Reload). Interne Zeitrechnung wie im Original in "Minuten seit
Mitternacht" — das hält die Portierung nachvollziehbar und diffbar.

**Hardware-Interface**:

```go
type Output interface {
    Apply(state uint16) error   // Bit 0 = Pumpe/Master, Bit 1..15 = Zonen
    Close() error
}
```

Backends in Phase 1: `mock` (Log-only, für Entwicklung auf Mac/PC), `script`
(externes Skript, kompatibel zum Original), `gpio` (direct pos/neg).
OpenSprinkler-Schieberegister: Phase 2.

**Weather-Interface**:

```go
type Provider interface {
    GetVals(ctx context.Context, s Settings) (ReturnVals, error)
}
```

`Scale(vals)` als freie Funktion mit exakt der Formel des Originals.
Provider wird zur **Laufzeit** in den Settings gewählt (nicht mehr zur Compile-Zeit).

### 2.4 REST-API (v1)

JSON-Bodies statt Query-Parameter, korrekte Verben, `Content-Type: application/json`:

| Neu                                   | Ersetzt                    |
|---------------------------------------|----------------------------|
| `GET  /api/state`                     | `json/state`               |
| `GET  /api/zones`                     | `json/zones`               |
| `PUT  /api/zones/{id}`                | `bin/setZones`             |
| `POST /api/zones/{id}/manual` `{"on":true}` | `bin/manual`         |
| `GET  /api/schedules`                 | `json/schedules`           |
| `POST /api/schedules`                 | `bin/setSched` (id=-1)     |
| `GET  /api/schedules/{id}`            | `json/schedule?id=`        |
| `PUT  /api/schedules/{id}`            | `bin/setSched`             |
| `DELETE /api/schedules/{id}`          | `bin/delSched`             |
| `POST /api/quickrun` `{"scheduleId":n}` oder `{"durations":[…]}` | `bin/setQSched` |
| `POST /api/stop`                      | (alle Zonen aus, Events löschen) |
| `PUT  /api/system/run` `{"enabled":true}` | `bin/run`             |
| `GET/PUT /api/settings`               | `json/settings`, `bin/settings` |
| `GET  /api/weather/check`             | `json/wcheck`              |
| `GET  /api/logs?start&end&group=none|hour|day|month` | `json/logs`, `json/tlogs` |

Zusätzlich: `GET /api/state` liefert genug für ein Live-Dashboard
(aktive Zone, Restzeit, nächste geplante Läufe); Polling alle 2–5 s reicht
(SSE/WebSocket erst, wenn nötig).

### 2.5 Web-Interface (Basis)

Seiten analog zum Original, aber als SPA:

1. **Dashboard** — Systemstatus, aktive Zone mit Countdown, Scheduler an/aus, Stop-Button
2. **Zonen** — Liste, umbenennen, aktivieren, Pumpen-Flag, manueller Start/Stopp je Zone
3. **Programme** — Liste mit "Nächster Lauf", Editor (Wochentage/Intervall, Restriktion, 4 Startzeiten, Dauer je Zone, Wetteranpassung an/aus)
4. **Schnellstart** — bestehendes Programm sofort starten oder Ad-hoc-Dauern
5. **Verlauf** — Tabelle + einfaches Balkendiagramm aus `/api/logs`
6. **Einstellungen** — Output-Typ, saisonale Anpassung, Wetter-Provider + Diagnose-Ansicht

---

## 3. Scope Phase 1 (Basisfunktionen)

**Enthalten:**
- Zonen- und Programmverwaltung (Datenmodell + API + UI)
- Scheduler-Engine mit Original-Semantik (§1.3), Quick Schedule, Manuellbetrieb, globaler Run-Schalter
- Hardware: mock, externes Skript, GPIO direct (pos/neg); Pumpe/Masterventil
- Logging nach SQLite + Verlaufs-API/-Ansicht (Gruppierung none/hour/day/month)
- Wetter: Interface, Dummy-Provider (100 %), Open-Meteo; saisonale Anpassung
- Settings-API/-UI, Wetter-Diagnose-Endpoint
- systemd-Unit, Cross-Compile-Doku für Raspberry Pi

**Bewusst NICHT in Phase 1:**
- Arduino/AVR-Support, NTP/statische Netzwerk-Config (Original-Altlast)
- OpenSprinkler-Schieberegister, GreenIQ, ChatterBox
- Weitere Wetter-Provider (OpenWeather, Aeris)
- Auth/HTTPS (Annahme wie Original: nur im LAN; hinter Reverse-Proxy möglich)
- Migration des EEPROM-Blobs (Neuanlage der Konfiguration; Zonen/Programme sind schnell erfasst)
- `bin/reset`/`bin/factory` (ersetzt durch systemd-Restart bzw. Löschen der config.json)

---

## 4. Meilensteine

| # | Meilenstein | Inhalt | Prüfbar durch |
|---|-------------|--------|---------------|
| M1 | Fundament | Go-Modul, Modelle + Validierung, config.json-Store (atomar), Logstore (SQLite-Schema) | Unit-Tests Store/Modelle |
| M2 | Engine | Eventlisten-Aufbau, Tick-Loop, RunState, Anpassungsformel, Quick/Manual/Stop | Tabellengetriebene Tests mit simulierter Uhr (Kern-Deliverable: Semantik-Parität) |
| M3 | Hardware | Output-Interface, mock + script + gpio, Pumpenlogik | mock-Backend in Engine-Tests; script-Backend manuell |
| M4 | API | Alle Endpoints aus §2.4, JSON-Fehlerformat | httptest-Suite; `curl`-Smoke-Test |
| M5 | Frontend | Vite-Setup, die 6 Seiten aus §2.5, embed ins Binary | Manuelle Prüfung im Browser (Desktop + Mobil) |
| M6 | Wetter + Deployment | Open-Meteo-Provider, Diagnose-Seite, systemd-Unit, README (Build, Cross-Compile, Installation) | Diagnose-Endpoint gegen echte API; Deployment auf Pi |

Reihenfolge-Logik: Die Engine (M2) ist das Risiko- und Korrektheitszentrum und wird
vor API/UI fertiggestellt und getestet — gegen simulierte Zeit, nicht Echtzeit.

---

## 5. Offene Punkte / Annahmen

1. **Sprache Go** angenommen (Projektverzeichnis heißt `sprinklerGo`).
2. **Zielplattform** Raspberry Pi (64-bit OS) angenommen; Entwicklung/Betrieb auf macOS mit mock-Backend möglich.
3. Max. 15 Zonen / 10 Programme werden als konfigurierbare Defaults übernommen (keine harten Compile-Zeit-Limits mehr).
4. Schedules über Mitternacht: zunächst wie Original (bekannte Einschränkung), sauberer Fix als Phase-2-Kandidat.
5. Anzeige 24-h-Format als UI-Einstellung statt Compile-Flag.
