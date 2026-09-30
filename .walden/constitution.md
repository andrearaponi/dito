# Project Constitution

This file captures stable project-wide context that applies across all features. It is optional and does not participate in the approval workflow.

## Project Summary

Dito è un reverse proxy L7 scritto in Go, con plugin firmati (Ed25519), hot reload della configurazione, metriche Prometheus e supporto WebSocket, pensato per il deploy su Kubernetes/OpenShift. Serve operatori che espongono backend HTTP e sviluppatori che estendono il proxy con middleware.

Il programma di remediation in corso è descritto in `SEPTEMBER-STATE.md`: finding `F-xx`, spec candidate `S-xx`, decisioni aperte `D-xx`, scenari di riproduzione `R-xx`. Dal 30/09/2026 è un documento di lavoro locale, non versionato e ignorato da git; l'ultima versione presente nel repository è quella del commit `c18a720`.

## Tech Stack

- Go 1.27.1 (`go.mod`), modulo `dito`.
- Binari: `cmd/` (proxy) e `cmd/plugin-signer` (generazione chiavi e firma dei plugin).
- Plugin: package `plugin` di Go (richiede CGO). Ogni plugin è un modulo separato in `plugins/<nome>/` con `replace dito => ../../`.
- Dipendenze principali: `prometheus/client_golang`, `gorilla/websocket`, `lmittmann/tint`, `fatih/color`, `gopkg.in/yaml.v3`; test con `stretchr/testify`.
- Container: builder `ubi8/go-toolset:1.26` con `GOTOOLCHAIN=go1.27.1`, runtime `ubi8/ubi-minimal`; manifest Kubernetes e OpenShift in `deployments/`.

## Conventions

- Un package per responsabilità alla radice del modulo: `app`, `config`, `handlers`, `middlewares`, `plugin`, `transport`, `websocket`, `writer`, `metrics`, `logging`.
- Branch `feature/…`, `fix/…`, `chore/…`; integrazione su `main` tramite pull request.
- Messaggi di commit in stile Conventional Commits (`feat:`, `fix:`, `refactor(scope):`, `build:`, `docs:`).
- Lingua: testo delle spec in italiano; parole chiave EARS, identificatori, codice e commenti nel codice in inglese.
- Ogni spec cita i finding di origine (*Fonte: SEPTEMBER-STATE F-xx*).

## Sanity Checks

Con `GOTOOLCHAIN=local` serve una toolchain Go ≥ 1.27.1 installata; in alternativa usare `GOTOOLCHAIN=go1.27.1`.

```bash
go build ./...
go vet ./...
(cd plugins/hello-plugin && go vet ./...)
go test -race -count=1 ./...   # verde dopo S-01 test-suite-hygiene
golangci-lint run ./...        # v2, compilato con Go >= 1.27
govulncheck ./...
```

## Key Files

- `SEPTEMBER-STATE.md` (locale, non versionato): stato del progetto e PRD di remediation.
- `cmd/main.go`: avvio, caricamento dei plugin, server HTTP, shutdown.
- `config/config.go`: schema della config, default, validazione, watcher dell'hot reload.
- `handlers/handlers.go`: match delle location e reverse proxy.
- `transport/transport.go`: transport HTTP per location e manipolazione degli header.
- `plugin/plugin.go`: interfaccia `Plugin`, verifica delle firme, caricamento.
- `middlewares/logging.go` e `writer/`: logging delle richieste e wrapper delle risposte.
- `cmd/config.yaml`: config di esempio; `configs/templates/`: template per Kubernetes.
- `Makefile`, `Dockerfile`, `deployments/`: build e deploy.

## Hard Rules

- Test ermetici: nessun test contatta la rete esterna; si usano `httptest` e il loopback.
- Spec di bugfix in TDD: il test di regressione deve fallire prima del fix.
- Nessuna nuova dipendenza senza approvazione esplicita.
- Ogni cambio di comportamento o di configurazione aggiorna il README e include note di migrazione.
- Default di sicurezza fail-closed.
- Un cambio all'interfaccia `plugin.Plugin` è un cambio di contratto: rompe i plugin esistenti.
- Host e plugin vanno compilati con la stessa toolchain Go, le stesse versioni delle dipendenze condivise e gli stessi flag di build.
- Commit solo su richiesta esplicita.
