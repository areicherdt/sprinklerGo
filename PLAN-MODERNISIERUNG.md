# Modernisierungsplan (Phase 2): sprinklerGo

> **Umsetzungsstand: alle Meilensteine M7–M12 umgesetzt, inkl. i18n.**
> Die zweisprachige Oberfläche (Deutsch/English, §3.B.6) wurde am 2026-07-08
> nachgezogen: zentrales Wörterbuch (`web/src/i18n.ts`) mit typgeprüften
> Schlüsseln, Sprachwahl in den Einstellungen (persistiert in der Config,
> v6), sprachabhängige Datums-/Wochentagsformate und Home-Assistant-
> Entitätsnamen; abgesichert durch einen E2E-Test für den Sprachwechsel.

Der Basis-Port (PLAN.md, M1–M6) hat die Original-Semantik von sprinklers_pi
bewusst 1:1 übernommen — inklusive einiger Konzepte von 2013. Dieser Plan
benennt, **was davon modernisiert werden sollte**, und bringt UI, Bewässerungs-
Workflow und Entwicklungs-/Betriebsprozesse auf aktuellen Stand.

---

## 1. Leitplanken

1. **Ein Binary bleibt:** Alle Erweiterungen laufen im bestehenden Prozess
   (kein separater Broker/DB-Server als Pflicht).
2. **Konfiguration bleibt migrierbar:** Jede Schema-Änderung erhöht
   `config.version` und wird beim Laden automatisch migriert.
3. **Engine-Parität bleibt testbar:** Die bestehenden Paritätstests bleiben
   bestehen; neue Semantik (z. B. Regenpause) wird über eigene Tests
   abgesichert und ist per Einstellung abschaltbar, wo sie Verhalten ändert.
4. **Raspberry Pi zuerst:** Ressourcenverbrauch und Offline-Fähigkeit im LAN
   haben Vorrang vor Cloud-Features.

---

## 2. Befund: Was ist heute noch „2013"?

### Bewässerungs-Workflow (Engine)

| Ist (geerbt vom Original) | Problem heute |
|---|---|
| Jede Konfigurationsänderung stoppt die laufende Bewässerung ([engine.Reload](internal/engine/engine.go)) | Zone umbenennen bricht den Morgenlauf ab — überraschend und verlustbehaftet |
| Manueller Betrieb läuft **unbegrenzt** bis zum Stopp | Vergessene Zone = Wasserverschwendung; moderne Controller haben Default-Timer |
| Keine Regenpause („Rain Delay") | Standard bei jedem aktuellen Controller (24/48/72 h aussetzen ohne Programm-Umbau) |
| Kein Zyklus & Sickern (Cycle & Soak) | Lange Laufzeiten erzeugen Oberflächenabfluss; Stand der Technik ist Aufteilen in Zyklen mit Sickerpausen |
| Programme über Mitternacht brechen ab (Original-TODO) | Bekannter Bug, im Port absichtlich beibehalten |
| Kollision = minutenweises Verschieben | Funktioniert, ist aber unsichtbar; eine explizite Warteschlange ist nachvollziehbarer |
| Kein Pumpen-Vor-/Nachlauf | Masterventil/Pumpe schaltet hart gleichzeitig mit der Zone |
| Wetterabruf blockiert synchron direkt vor Programmstart | Bei API-Hängern verzögert sich der Tick; kein Cache, keine Anzeige des Werts *vor* dem Lauf |
| Saisonale Anpassung = ein globaler Prozentwert | Üblich ist ein Monatsprofil (Jan–Dez) oder ET-basierte Anpassung |

### UI & Feedback

| Ist | Problem heute |
|---|---|
| Polling alle 3 s ([usePoll](web/src/util.ts)) | Verzögertes Feedback, unnötige Requests; Stand der Technik: Server-Push (SSE) |
| `window.confirm`/`alert` ([Schedules.tsx](web/src/pages/Schedules.tsx)) | Native Browser-Dialoge statt konsistenter UI |
| Kein Tagesplan („Was läuft heute wann?") | `pendingEvents` ist nur eine Zahl; moderne Controller zeigen eine Timeline |
| Kein Fortschrittsbalken des laufenden Programms | Nur Restzeit der aktuellen Zone, kein Gesamtüberblick |
| `clock24h`-Einstellung wird gespeichert, aber im UI nicht angewendet | Umsetzungslücke aus M5 |
| Nur Deutsch, hart kodiert | i18n-Grundlage fehlt (DE/EN) |
| Kein PWA-Manifest | Auf dem Handy nicht „installierbar", kein Homescreen-Icon |
| Keine A11y-Prüfung, kein CSV-Export im Verlauf | Kleinere Lücken |

### Prozesse (Entwicklung, Betrieb, Integration)

| Ist | Problem heute |
|---|---|
| **Kein Git-Repository**, keine CI | Kein Review-/Release-Prozess, keine reproduzierbaren Builds |
| Version als Konstante in [main.go](cmd/sprinklerd/main.go) | Kein Bezug zu Tag/Commit; Releases nicht nachvollziehbar |
| Keine API-Spezifikation | Endpoints nur im README beschrieben; keine generierbaren Clients |
| Kein Auth, kein HTTPS-Konzept | LAN-Annahme wie 2013; heute erwartet man mindestens optionalen Login/API-Token |
| `zonelog` wächst unbegrenzt | Keine Aufbewahrungsrichtlinie/VACUUM ([logstore.go](internal/store/logstore.go)) |
| Kein Backup/Restore der Konfiguration | Nur manuelles Kopieren der config.json |
| Keine Smart-Home-Anbindung | MQTT/Home Assistant ist heute die Standard-Erwartung für Geräte im Haus |
| Deployment nur als Binary + systemd | Docker (multi-arch) fehlt als Alternative; keine Release-Artefakte |

---

## 3. Maßnahmen nach Themenfeld

### A. Workflow-Modernisierung (Engine)

1. **Regenpause:** `POST /api/rain-delay {hours}` + Countdown im Dashboard;
   unterdrückt Programmstarts, manuelle Läufe bleiben möglich. Persistiert
   in config.json (übersteht Neustart).
2. **Manueller Timer:** Manuelle Läufe bekommen eine Default-Dauer
   (einstellbar, z. B. 30 min, „unbegrenzt" bleibt wählbar). UI zeigt Countdown.
3. **Sanfter Reload:** Konfigurationsänderungen bauen nur die *anstehenden*
   Events neu; ein laufender Zyklus läuft mit den alten Werten zu Ende.
   Stopp nur noch bei Änderungen, die den laufenden Zyklus selbst betreffen.
4. **Mitternachts-Fix:** Events tragen ein Datum statt nur Minuten-im-Tag;
   Läufe über 0:00 Uhr laufen durch (behebt das Original-TODO).
5. **Zyklus & Sickern:** Optional je Programm: max. Zyklusdauer + Sickerpause;
   die Engine teilt Zonenlaufzeiten automatisch auf.
6. **Pumpen-Vor-/Nachlauf:** Konfigurierbare Sekunden zwischen Master-/
   Pumpenausgang und Zonenventil.
7. **Warteschlange statt Minuten-Defer:** Kollidierende Programme landen in
   einer sichtbaren Queue (API + UI), Reihenfolge nachvollziehbar.
8. **Wetter entkoppeln:** Abruf asynchron mit Cache (z. B. stündlich, TTL);
   die Engine liest nur den Cache. Dashboard zeigt die aktuell wirksame
   Skalierung *bevor* ein Programm startet, inkl. Zeitstempel des Abrufs.
9. **Monatsprofil:** Saisonale Anpassung wahlweise als 12-Monats-Kurve
   (das globale Prozentfeld bleibt als einfacher Modus). ET-basierte
   Anpassung wurde bewusst verworfen (Entscheidung, siehe §7).

### B. UI-Modernisierung

1. **Live-Updates per SSE:** `GET /api/events` (Server-Sent Events) für
   Status/Zonenwechsel; Polling nur als Fallback. Reaktionszeit < 1 s.
2. **Dashboard-Ausbau:** Tages-Timeline (geplant/gelaufen/laufend),
   Fortschrittsbalken über alle Zonen des aktiven Programms, Regenpause-
   Kachel, wirksame Wetter-Skalierung.
3. **Programm-Editor:** Live-Vorschau „nächste Läufe" beim Editieren,
   Gesamtlaufzeit-Summe, Stepper/Slider für Laufzeiten, Duplizieren von
   Programmen.
4. **Dialog-/Toast-System:** Eigene Bestätigungsdialoge und Erfolgs-/
   Fehler-Toasts statt `window.confirm`/`alert`; einheitliches Fehlerbild.
5. **Kleinigkeiten mit Wirkung:** `clock24h` überall anwenden, CSV-Export im
   Verlauf, Zonen sortierbar, leere Zustände mit Handlungsaufforderung.
6. **i18n:** Texte in Sprachdateien (DE vollständig, EN nachziehen);
   Sprachwahl in den Einstellungen.
7. **PWA:** Manifest + Service Worker (App-Shell-Cache, Status read-only
   offline), Homescreen-Installation auf iOS/Android.
8. **A11y-Pass:** Fokusreihenfolge, ARIA-Labels, Kontraste (Chart-Palette ist
   bereits validiert), Bedienbarkeit ohne Maus.

### C. Schnittstellen & Integration

1. **OpenAPI 3.1-Spezifikation** für die gesamte API (Datei im Repo +
   `GET /api/openapi.json`); Beispiele im README ersetzen.
2. **MQTT + Home Assistant Discovery:** Optionaler MQTT-Client in den
   Einstellungen; publiziert Zonenzustände/Status, abonniert Kommandos
   (Zone an/aus, Stopp, Regenpause). HA-Discovery-Topics, damit Zonen als
   Switches/Sensoren automatisch erscheinen.
3. **Benachrichtigungen/Webhooks:** Ereignisse (Lauf beendet, Fehler,
   Wetterabruf fehlgeschlagen) an konfigurierbare Webhook-URLs; Grundlage
   für ntfy/Telegram/E-Mail.
4. **Backup/Restore im UI:** Export/Import der config.json (mit Versions-
   und Plausibilitätsprüfung) — ersetzt das alte `bin/factory` sauber.

### D. Sicherheit & Betrieb

1. **Optionale Authentifizierung:** Login mit Passwort (Session-Cookie) +
   API-Tokens für Automatisierung; standardmäßig aus (LAN), in den
   Einstellungen aktivierbar. Schreibende Endpoints erfordern dann Auth.
2. **HTTPS-Leitfaden:** Dokumentierter Reverse-Proxy-Weg (Caddy/Traefik)
   statt eigener TLS-Implementierung.
3. **Log-Retention:** Aufbewahrung konfigurierbar (z. B. 24 Monate),
   periodisches Aufräumen + VACUUM.
4. **Docker-Image (multi-arch amd64/arm64)** als Deployment-Alternative zur
   systemd-Unit; Volumes für config/db, GPIO-Durchreichung dokumentiert.
5. **Metrics (optional):** `GET /metrics` (Prometheus) hinter Feature-Flag —
   Laufzeiten, Fehler, Wetter-Skalierung.

### E. Entwicklungsprozesse

1. **Git-Repository initialisieren** (das Verzeichnis ist noch keines!) mit
   sinnvoller .gitignore (bin/, web/node_modules, web/dist, *.db).
2. **CI (GitHub Actions):** `go vet` + `golangci-lint` + `go test` +
   Frontend-Typecheck/Build bei jedem Push; Release-Workflow baut
   arm64/amd64-Binaries und Docker-Images bei Tags.
3. **Version per ldflags** (`-X main.version=$(git describe)`) statt
   Konstante; sichtbar in UI-Footer und `/api/state`.
4. **Frontend-Qualität:** ESLint + Prettier, `npm run check` in CI.
5. **E2E-Tests (Playwright):** Die 5 Kernflüsse — Programm anlegen,
   Schnellstart, manuelle Zone, Einstellungen speichern, Regenpause —
   gegen das echte Binary mit Mock-Ausgang.
6. **Config-Migrationen:** `version`-Feld auswerten, Migrationskette mit
   Tests (erstmals nötig für Monatsprofil/Cycle&Soak-Felder).

---

## 4. Meilensteine (Fortsetzung der Nummerierung)

| # | Meilenstein | Inhalt | Größe |
|---|---|---|---|
| **M7** | Prozess-Fundament | Git init, CI mit Lint/Test/Build, ldflags-Version, ESLint/Prettier, OpenAPI-Spec, Config-Migrationsgerüst, Log-Retention | M |
| **M8** | Workflow-Kern | Regenpause, manueller Timer, sanfter Reload, Mitternachts-Fix, Wetter-Cache (async) — je mit Engine-Tests | L |
| **M9** | UI-Modernisierung | SSE-Live-Updates, Dashboard-Timeline + Fortschritt, Dialog/Toast-System, Editor-Vorschau, clock24h, CSV-Export | L |
| **M10** | Bewässerungs-Feinheiten | Zyklus & Sickern, Pumpen-Vor-/Nachlauf, Kollisions-Queue, Monatsprofil | M |
| **M11** | Integration | MQTT + HA-Discovery, Webhooks, Backup/Restore im UI | M |
| **M12** | Sicherheit & Deployment | Optionale Auth + API-Tokens, Docker multi-arch, HTTPS-Doku, Release-Pipeline, E2E-Tests, i18n/PWA/A11y | L |

Reihenfolge (nach den Entscheidungen in §7): **M7 → M8 → M9 → M11 → M10 → M12.**
M7 zuerst (billig, senkt Risiko für alles Weitere), dann M8/M9 als spürbarster
Alltagsnutzen. M11 (MQTT/Home Assistant) ist gewünscht und wird vor M10
gezogen — es baut auf dem Ereignis-Bus auf, den M8/M9 ohnehin einführen
(Regenpause-Status, SSE). M9 hängt teilweise an M8 (SSE transportiert die
neuen Zustände).

## 5. Nicht-Ziele (bewusst außen vor)

- Cloud-Dienst/Fernzugriff als eigener Service (Reverse-Proxy/VPN genügt)
- Multi-Controller-Verwaltung
- Durchflusssensorik/Leckerkennung (neue Hardware; API-seitig offen halten)
- Native Apps (PWA deckt Mobile ab)

## 6. Risiken & Absicherung

- **Semantik-Drift der Engine:** Neue Features (sanfter Reload, Mitternacht,
  Cycle&Soak) ändern bewusst Original-Verhalten → jede Änderung bekommt
  eigene tabellengetriebene Tests; die Paritätstests bleiben für den
  unveränderten Kern bestehen.
- **Config-Kompatibilität:** Migrationskette ab `version: 1` mit Tests je
  Migrationsschritt; Backup-Export vor Migration im UI empfohlen.
- **MQTT/Auth als optionale Pfade:** standardmäßig deaktiviert, damit das
  Basis-Setup (ein Binary im LAN) unverändert einfach bleibt.

## 7. Entscheidungen (2026-07-07)

1. **MQTT/Home Assistant: JA** — M11 wird vor M10 gezogen (siehe §4).
2. **Auth: JA**, als optionaler Single-User-Login + API-Tokens (kein Multi-User).
3. **ET-basierte Bewässerung: NEIN** — gestrichen; es bleibt bei
   Wetter-Formel + Monatsprofil.
4. **Hosting: GitHub** — CI/Release über GitHub Actions wie in §3.E geplant.
   Offene Detailfragen für M7: Repo-Name, Organisation/Account, public/private.
