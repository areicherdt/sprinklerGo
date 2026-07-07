# sprinklerGo

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
- Wetter-Anpassung über **Open-Meteo** (kostenlos, kein API-Key) mit Diagnose-Seite
- Regenpause, Timer für manuelle Läufe, Live-Updates per SSE
- **MQTT mit Home-Assistant-Discovery** (Zonen als Schalter, Automatik, Regenpause,
  Stopp-Taste, Sensoren), Webhook-Benachrichtigungen, Backup/Restore im UI
- Hardware-Backends: `none` (Test), externes Skript (kompatibel zum Original),
  GPIO direkt aktiv-high/-low (Linux gpiochip)
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

```sh
make arm64
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
Für GPIO-Ausgänge braucht der Dienst Zugriff auf `/dev/gpiochip0` — die Unit
hängt den Nutzer dafür an die Gruppe `gpio`. Logs: `journalctl -u sprinklerd -f`.

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
```

CI (GitHub Actions) prüft bei jedem Push: Frontend-Check und -Build,
`go vet`, `golangci-lint`, alle Tests und den arm64-Cross-Compile. Ein
`v*`-Tag baut Release-Binaries (linux/arm64 + amd64) und veröffentlicht sie
als GitHub-Release.

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
| `GET /api/weather/check` | Wetter-Diagnose |
| `GET /api/logs?group=none|hour|day|month&start=&end=` | Verlauf |

Der Verlauf wird gemäß der Einstellung „Verlauf aufbewahren" automatisch
bereinigt (Standard: 24 Monate, 0 = unbegrenzt).

## Lizenz

GPL-2.0 — sprinklerGo ist ein Port von
[sprinklers_pi](https://github.com/rszimm/sprinklers_pi)
(© Richard Zimmerman) und übernimmt dessen Lizenz. Siehe [LICENSE](LICENSE).
