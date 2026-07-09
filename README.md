# sprinklerGo

[![CI](https://github.com/areicherdt/sprinklerGo/actions/workflows/ci.yml/badge.svg)](https://github.com/areicherdt/sprinklerGo/actions/workflows/ci.yml)

Go-Port von [sprinklers_pi](https://github.com/rszimm/sprinklers_pi) — eine
Bewässerungssteuerung für den Raspberry Pi mit REST-API und modernem Web-Interface
in einem einzigen Binary. Analyse und Architektur: [PLAN.md](PLAN.md).

**Status:** Alle Meilensteine des Basis-Plans (M1–M6) umgesetzt.
Die Modernisierung von Workflow, UI und Prozessen (M7–M12) ist in
[PLAN-MODERNISIERUNG.md](PLAN-MODERNISIERUNG.md) geplant.

## Funktionen

- Bis zu 15 Zonen (plus Pumpen-/Masterventil-Ausgang), bis zu 50 Programme
- Programme mit Wochentagen **oder** Intervall, Gerade/Ungerade-Restriktion,
  bis zu 4 Startzeiten, Laufzeit je Zone, saisonale + Wetter-Anpassung
- Scheduler mit der Original-Semantik (sequentielle Zonen, Mitternachts-Reload,
  minutenweise Verschiebung bei Kollisionen)
- Manuellbetrieb, Schnellstart (Programm sofort oder Ad-hoc-Laufzeiten)
- Verlauf in SQLite mit Tabellen- und Diagramm-Ansicht
- Wetter-Anpassung über **Open-Meteo** (kostenlos, kein API-Key) oder
  **OpenWeather** (One Call 3.0, API-Key nötig), mit Diagnose-Seite
- Optionale **Prometheus-Metriken** unter `/metrics`
- Regenpause, Timer für manuelle Läufe, Live-Updates per SSE
- **MQTT mit Home-Assistant-Discovery** (Zonen als Schalter, Automatik, Regenpause,
  Stopp-Taste, Sensoren), Webhook-Benachrichtigungen, Backup/Restore im UI
- Zyklus & Sickern, Pumpen-Vor-/Nachlauf, Monatsprofil für die saisonale Anpassung
- Optionaler Login mit API-Tokens; als PWA auf dem Homescreen installierbar
- Zweisprachige Oberfläche (Deutsch/English), umschaltbar in den Einstellungen
- Hardware-Backends: `none` (Test), externes Skript (kompatibel zum Original),
  GPIO direkt aktiv-high/-low (Linux gpiochip), **GreenIQ Gen2** (feste Pin-Map)
- REST-API (`/api/...`) + eingebettetes React-Frontend (deutsch, hell/dunkel)

## Bauen

Voraussetzungen: Go ≥ 1.24, Node ≥ 20.

```sh
make            # Frontend + Binary für die aktuelle Plattform (bin/sprinklerd)
make arm64      # Cross-Compile für Raspberry Pi 64-bit (bin/sprinklerd-arm64)
make test       # go vet + alle Tests
```

Ohne make: erst `cd web && npm install && npm run build`, dann
`go build -o sprinklerd ./cmd/sprinklerd` (das Frontend wird per `go:embed`
eingebettet).

## Starten

```sh
./bin/sprinklerd -config config.json -db zonelog.db
```

Beim ersten Start wird `config.json` mit Werksdefaults angelegt (Port 8080,
Scheduler aus, nur Zone 1 aktiv). Web-UI: `http://<host>:8080`.

Flags: `-config` Konfigurationsdatei · `-db` Log-Datenbank · `-port` Port-Override ·
`-debug` Debug-Logging · `-version`.

## Installation auf dem Raspberry Pi (systemd)

**Welches Binary?** Auf dem Pi `uname -m` ausführen: `aarch64` →
`sprinklerd-linux-arm64`; `armv7l` oder `armv6l` (32-bit Raspberry Pi OS) →
`sprinklerd-linux-armv6`. Ein arm64-Binary auf einem 32-bit-System scheitert
mit „Exec format error".

```sh
make arm64        # bzw. make armv6 für 32-bit Raspberry Pi OS
scp bin/sprinklerd-arm64 pi@<host>:/tmp/sprinklerd
scp deploy/sprinklerd.service pi@<host>:/tmp/

# auf dem Pi:
sudo useradd --system --home /var/lib/sprinklerd --groups gpio sprinklerd
sudo install -m 755 /tmp/sprinklerd /usr/local/bin/sprinklerd
sudo install -m 644 /tmp/sprinklerd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sprinklerd
```

Konfiguration und Log-Datenbank liegen dann unter `/var/lib/sprinklerd/`.

**Web-Port ändern:** Ein neuer Port (Einstellungen → System) greift sofort —
der Server zieht ohne Neustart um, die Oberfläche verlinkt die neue Adresse.
Schlägt das Binden fehl (Port belegt oder ohne Berechtigung), bleibt der alte
Port aktiv und die Einstellung wird zurückgesetzt. Für **privilegierte Ports
wie 80** braucht der Dienst `AmbientCapabilities=CAP_NET_BIND_SERVICE` — in
der mitgelieferten Unit ab v0.8.2 enthalten; bei älteren Installationen die
Unit-Datei aktualisieren und `sudo systemctl daemon-reload && sudo systemctl
restart sprinklerd` ausführen.
Für GPIO-Ausgänge braucht der Dienst Zugriff auf `/dev/gpiochip0` — die Unit
hängt den Nutzer dafür an die Gruppe `gpio`. Logs: `journalctl -u sprinklerd -f`.

### GreenIQ Gen2

Einfach als Ausgabetyp „GreenIQ Gen2" wählen — die Pin-Belegung (Masterventil
plus 6 Zonen auf Kabel 1, Direct Positive/aktiv-high) ist fest hinterlegt und
entspricht dem `#define GREENIQ` des Originals; es müssen keine GPIO-Pins von
Hand eingetragen werden. Verdrahtet sind Zone 1–6. Sensoren auf Kabel 2 werden
wie im Original nicht unterstützt.

## Wetter-Anpassung einrichten

1. Einstellungen → Wetter → Anbieter „Open-Meteo" wählen (kein API-Key nötig).
2. Standort als `Breitengrad,Längengrad` eintragen (z. B. `52.52,13.40`).
3. Speichern und „Wetter-Diagnose ausführen" klicken — sie zeigt die
   abgerufenen Werte und die resultierende Skalierung (0–200 %).
4. In den Programmen „Wetter-Anpassung" aktivieren.

Die Skalierung wird wie im Original unmittelbar vor jedem Programmstart
berechnet: `100 + Feuchte-Faktor + Temperatur-Faktor + Regen-Faktor` aus den
Vortageswerten, multipliziert mit der saisonalen Anpassung.

## Entwicklung

```sh
go test ./...                 # Backend-Tests (Engine mit simulierter Uhr)
cd web && npm run dev         # Frontend-Dev-Server, proxied /api auf :8080
cd web && npm run check       # TypeScript + ESLint + Prettier
go build -o bin/sprinklerd-e2e ./cmd/sprinklerd && cd web && npm run e2e
                              # Playwright-E2E gegen das echte Binary
```

CI (GitHub Actions) prüft bei jedem Push: Frontend-Check und -Build,
`go vet`, `golangci-lint`, alle Tests und den arm64-Cross-Compile. Ein
`v*`-Tag baut Release-Binaries (linux/arm64 + amd64) und veröffentlicht sie
als GitHub-Release.

## Docker

Alternativ zum Binary gibt es ein Multi-Arch-Image (amd64/arm64) auf ghcr.io:

```sh
docker run -d --name sprinklerd \
  -p 8080:8080 \
  -v sprinklerd-data:/data \
  -e TZ=Europe/Vienna \
  ghcr.io/areicherdt/sprinklergo:latest
```

Für GPIO-Ausgänge zusätzlich `--device /dev/gpiochip0 --group-add $(getent group gpio | cut -d: -f3)`.
Die Zeitzone (`TZ`) bestimmt die Startzeiten der Programme.

## Sicherheit

Standardmäßig ist die API offen (LAN-Annahme wie beim Original). Unter
Einstellungen → Sicherheit lässt sich ein Passwort setzen und die Anmeldung
aktivieren: Die Weboberfläche verlangt dann einen Login (Session-Cookie,
30 Tage), API-Aufrufe brauchen ein Token (`Authorization: Bearer <token>`),
das dort erzeugt und widerrufen werden kann.

Für Zugriff von außerhalb des LANs empfiehlt sich ein Reverse-Proxy mit
HTTPS, z. B. Caddy:

```
sprinkler.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## Monitoring (Prometheus)

Unter Einstellungen → System „Prometheus-Metriken bereitstellen" aktivieren.
Dann liefert `GET /metrics` u. a. `sprinklergo_runs_started_total`,
`sprinklergo_runs_finished_total`, `sprinklergo_errors_total{kind=…}`,
`sprinklergo_weather_scale_percent`, `sprinklergo_scheduler_enabled` und
`sprinklergo_active_zone`. Ist die Anmeldung aktiv, braucht der Scraper ein
API-Token:

```yaml
scrape_configs:
  - job_name: sprinklergo
    static_configs:
      - targets: ['sprinkler.local:8080']
    authorization:
      credentials: '<API-Token>'
```

## Home Assistant anbinden

1. Einstellungen → Integration → MQTT aktivieren, Broker eintragen
   (z. B. `tcp://homeassistant.local:1883`), speichern.
2. Bei aktivierter Discovery erscheint das Gerät „sprinklerGo" automatisch in
   Home Assistant: jede aktive Zone als Schalter (Einschalten startet mit dem
   manuellen Timer), dazu Automatik- und Regenpause-Schalter, Stopp-Taste und
   Sensoren für aktive Zone und Wetter-Skalierung.
3. Topics liegen unter dem konfigurierten Präfix (`sprinklergo/...`),
   Kommandos auf `.../set`, Verfügbarkeit auf `sprinklergo/availability`.

## API-Kurzreferenz

Vollständige Spezifikation: [internal/api/openapi.json](internal/api/openapi.json),
zur Laufzeit unter `GET /api/openapi.json`.

| Endpoint | Zweck |
|---|---|
| `GET /api/state` | Systemstatus (aktive Zone, Restzeit, …) |
| `GET /api/zones` · `PUT /api/zones/{id}` | Zonen lesen/ändern |
| `POST /api/zones/{id}/manual` `{"on":true}` | Zone manuell an/aus |
| `GET/POST /api/schedules` · `GET/PUT/DELETE /api/schedules/{id}` | Programme |
| `POST /api/quickrun` `{"scheduleId":0}` oder `{"durations":[5,0,3]}` | Schnellstart |
| `POST /api/stop` | Alles stoppen |
| `PUT /api/system/run` `{"enabled":true}` | Scheduler global an/aus |
| `GET/PUT /api/settings` | Einstellungen |
| `PUT /api/rain-delay` `{"hours":24}` | Regenpause setzen/aufheben (0 = aus) |
| `GET /api/events` | Live-Status als Server-Sent Events |
| `GET /api/weather/check` | Wetter-Diagnose |
| `GET /api/logs?group=none|hour|day|month&start=&end=` | Verlauf |
| `GET /api/backup` · `POST /api/restore` | Konfiguration sichern/wiederherstellen |
| `GET/PUT /api/auth` · `/api/auth/login|logout|password|tokens` | Anmeldung & API-Tokens |

Der Verlauf wird gemäß der Einstellung „Verlauf aufbewahren" automatisch
bereinigt (Standard: 24 Monate, 0 = unbegrenzt).

## Lizenz

GPL-2.0 — sprinklerGo ist ein Port von
[sprinklers_pi](https://github.com/rszimm/sprinklers_pi)
(© Richard Zimmerman) und übernimmt dessen Lizenz. Siehe [LICENSE](LICENSE).
