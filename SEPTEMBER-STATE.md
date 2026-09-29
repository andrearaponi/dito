# SEPTEMBER-STATE — Dito: stato del progetto e PRD di remediation

> Documento di lavoro (settembre 2026). Raccoglie le evidenze del deep dive del 29/09/2026 e le organizza in un portfolio di spec Walden.
> **Non è un contratto**: il contratto di ogni area nasce dalla relativa spec Walden approvata.

| Campo | Valore |
|---|---|
| Data | 2026-09-29 |
| Stato | Bozza v1, da revisionare |
| Owner | Andrea Raponi |
| Baseline analizzata | `main` @ `3c6c63f` (2025-06-19) + branch `chore/go-1.27` (`bd9325c` upgrade, `5838408` `go fix`) |
| Toolchain di riferimento | Go 1.27.1 |
| Riferimenti di riga | relativi al commit `5838408` (branch `chore/go-1.27`, dopo `go fix`) |
| Destinazione | backlog → spec Walden in `.walden/specs/<feature>/` |

## Indice

1. [Come usare questo documento](#1-come-usare-questo-documento)
2. [Executive summary](#2-executive-summary)
3. [Contesto, obiettivi, non-obiettivi](#3-contesto-obiettivi-non-obiettivi)
4. [Stato attuale](#4-stato-attuale)
5. [Lavoro già fatto: baseline Go 1.27.1](#5-lavoro-già-fatto-baseline-go-1271)
6. [Registro delle evidenze](#6-registro-delle-evidenze)
7. [Dettaglio dei finding critici e alti](#7-dettaglio-dei-finding-critici-e-alti)
8. [Portfolio delle spec Walden](#8-portfolio-delle-spec-walden)
9. [Decisioni aperte](#9-decisioni-aperte)
10. [Criteri di successo](#10-criteri-di-successo)
11. [Governance e bootstrap Walden](#11-governance-e-bootstrap-walden)
- [Appendice A — Log delle verifiche](#appendice-a--log-delle-verifiche)
- [Appendice B — Vulnerabilità con la toolchain 1.24.0](#appendice-b--vulnerabilità-con-la-toolchain-1240)
- [Appendice C — Scenari di riproduzione](#appendice-c--scenari-di-riproduzione)
- [Appendice D — Analisi statica](#appendice-d--analisi-statica)
- [Appendice E — Sospetti smentiti e limiti dell'analisi](#appendice-e--sospetti-smentiti-e-limiti-dellanalisi)

---

## 1. Come usare questo documento

- **ID stabili**: `F-xx` finding, `S-xx` spec candidate, `D-xx` decisioni aperte, `R-xx` scenari di riproduzione, `K-x` criteri di successo. Non si rinumerano: i requirements Walden citano la fonte (es. *Fonte: SEPTEMBER-STATE F-01*). Nuovi finding si aggiungono in coda con un nuovo ID.
- **Flusso per ogni spec**: `walden feature init <nome>` → requirements EARS (i "temi di accettazione" di §8.3 sono il punto di partenza, non il contratto) → design → tasks con proof → esecuzione solo su richiesta esplicita.
- **Bugfix in TDD**: nelle spec che ripristinano un comportamento, il primo task trasforma lo scenario `R-xx` in un test di regressione e lo vede fallire prima del fix.
- **Aggiornamenti**: durante il programma questo documento cambia solo nella tabella di tracking (§8.4), nelle decisioni chiuse (§9) e con eventuali nuovi finding.

**Gravità**

| Simbolo | Significato |
|---|---|
| 🔴 Critico | perdita o alterazione di dati, bypass di sicurezza, race in uso normale |
| 🟠 Alto | comportamento errato visibile o rischio concreto di sicurezza/affidabilità |
| 🟡 Medio | impatto limitato o condizionato; debito che amplifica altri rischi |
| 🔵 Basso | igiene, documentazione, tooling |

**Tipo di evidenza**

| Tag | Significato |
|---|---|
| `REPRO` | riprodotto con test automatico (harness temporaneo, Appendice C) |
| `E2E` | riprodotto con il binario o l'immagine reali |
| `RACE` | segnalato dal race detector |
| `LINT` | analisi statica (golangci-lint 2.14.0, govulncheck) |
| `MISURA` | misurato (copertura, conteggi) |
| `OSS` | osservato su repository, registry o processo |
| `CODE` | lettura del codice, non eseguito |

---

## 2. Executive summary

- Dito è un reverse proxy L7 in Go (≈3.900 righe di produzione, ≈2.300 di test) con plugin firmati, hot reload, metriche Prometheus e WebSocket. Ultimo commit su `main`: 19/06/2025.
- **Baseline aggiornata** a Go **1.27.1** con dipendenze aggiornate e immagine verificata: vulnerabilità note raggiungibili **31 → 0** (§5).
- **50 finding** (49 dal deep dive, 1 emerso durante S-01): 4 🔴, 13 🟠, 23 🟡, 10 🔵. Tutti i 🔴 sono stati riprodotti.
- I 4 critici:
  - risposte troncate senza errore (F-01);
  - POST di form rotti (F-02);
  - WebSocket che salta l'autenticazione dei plugin (F-13);
  - data race durante l'hot reload (F-23).
- Pattern ricorrenti:
  1. stato condiviso letto senza sincronizzazione e due fonti di verità per la config;
  2. strati di wrapper e buffer che alterano il data path;
  3. default fail-open;
  4. assenza di CI: nessuno di questi bug poteva essere intercettato.
- Il rischio strategico principale è il modello di plugin basato sul package `plugin` di Go: ogni upgrade di Go rompe i plugin di terze parti (F-49).
- **Proposta**: 14 spec Walden in 5 ondate (§8). Si parte da test ermetici e CI (ondata 0), così ogni proof successiva può contare su `go test -race`.

---

## 3. Contesto, obiettivi, non-obiettivi

**Obiettivi del programma**

| ID | Obiettivo |
|---|---|
| G-1 | Data path corretto: nessuna perdita o alterazione di richieste e risposte |
| G-2 | Sicurezza fail-closed: nessun bypass dei middleware, default sicuri, segreti fuori dai pod |
| G-3 | Hot reload consistente: nessuna race, semantica definita per le richieste in volo |
| G-4 | Osservabilità affidabile: metriche esatte, log strutturati e senza dati sensibili |
| G-5 | Manutenibilità: CI con gate (race, lint, vulnerabilità), test ermetici, toolchain aggiornata |
| G-6 | Modello di estensione che sopravviva agli upgrade di Go |

**Non-obiettivi** (salvo decisione esplicita): nuove funzionalità (rate limiting, cache, compressione: vedi D-14), migrazione della documentazione a mdBook, HTTP/3, API di amministrazione.

**Attori**: operatore (deploy e configurazione), sviluppatore di plugin, client delle API proxate, maintainer.

---

## 4. Stato attuale

### 4.1 Numeri

| Voce | Valore |
|---|---|
| Release | 0.7.5 |
| Commit | 48, dal 2024-10-09 al 2025-06-19 |
| Branch non mergiati | 3, solo modifiche al README del 2024 |
| Codice | ≈3.900 righe di produzione, ≈2.300 di test |
| Go | `go 1.23.2` su `main`, `go 1.27.1` sul branch `chore/go-1.27` |
| CI | assente (`.github/` contiene solo `FUNDING.yml`) |
| Analisi qualità | SonarQube solo in locale |

### 4.2 Pipeline di una richiesta (com'è oggi)

```
client
 └─ http.Server (nessun timeout)                                        cmd/main.go:126
     └─ mux "/" → LoggingMiddleware                                      middlewares/logging.go:92
         ├─ legge 1 KB del body per il log e ricostruisce r.Body
         ├─ writer.ResponseWriter #1 (tee-buffer fino a 512 KB)
         └─ DynamicProxyHandler                                           handlers/handlers.go:79
             ├─ validateRequest: Content-Length ≤ 10 MB, r.ParseForm()       :644
             ├─ path == metrics.path → promhttp
             ├─ match regex sulle location (dito.Config letto senza lock)
             │   ├─ WebSocket → proxy gorilla, SENZA middleware              :98
             │   └─ handleLocationMatch                                     :123
             │       ├─ writer.ResponseWriter #2 (tee-buffer fino a 512 KB)
             │       ├─ responseLimitInterceptor (limite di default 100 MB)
             │       ├─ middleware dei plugin
             │       └─ timeout → ServeProxy → ReverseProxy NUOVO per richiesta
             │           (Director deprecato, BufferPool nuovo) → Caronte.RoundTrip
             │           (header, X-Forwarded-*, TransportCache)
             └─ nessun match → 404 JSON
```

- **Hot reload**: `WatchConfig` (polling ogni 2 s) → `LoadConfiguration` → confronto con la config *globale*, che non viene mai aggiornata → `UpdateComponents` + `UpdateConfig` → `TransportCache.Clear()`.
- **Plugin**: all'avvio verifica l'hash della chiave pubblica → firma ed25519 di sha256(`.so`) → `plugin.Open` → `NewPlugin()` → `Init`. Richiede CGO e la stessa toolchain tra host e plugin.

### 4.3 Test e copertura (Go 1.27.1)

| Package | Copertura | Note |
|---|---|---|
| app | 47,4% | |
| config | 79,5% | `TestWatchConfig` ha una race nel test stesso |
| handlers | 46,5% | `TestDynamicProxyHandler` chiama `http://example.com` |
| logging | 97,1% | |
| metrics | 58,5% | |
| transport | 33,7% | |
| writer | 79,8% | race reale in `LimitedBuffer.Read`; `TestConcurrentWrites` usa un contratto non valido |
| cmd, middlewares, plugin, websocket | 0% | nessun test |

Senza `-race` tutti i test passano; con `-race` ne falliscono 3 (F-27, F-41).

### 4.4 Vulnerabilità

- Toolchain 1.24.0: **31 vulnerabilità della libreria standard raggiungibili** (Appendice B), più 8 in package importati e 16 in moduli richiesti ma non raggiungibili.
- Dopo l'upgrade a 1.27.1: `govulncheck ./...` → *No vulnerabilities found*.

### 4.5 Analisi statica

golangci-lint 2.14.0, con in più gosec, bodyclose, errorlint, noctx, modernize, unparam, contextcheck, nilerr e copyloopvar: **105 issue** (Appendice D).

### 4.6 Infrastruttura

- Dockerfile multi-stage UBI 8 (builder go-toolset, runtime ubi-minimal), CGO attivo per i plugin, utente non root, HEALTHCHECK su `/metrics`.
- Manifest Kubernetes e OpenShift con probe su `/metrics`; tag immagine `v1.1.0-production` e `v2.0.0-production`, non allineati alla release 0.7.5.
- Makefile ricco di target per setup, chiavi e OpenShift; mancano target per lint, race, vulnerabilità e copertura.

---

## 5. Lavoro già fatto: baseline Go 1.27.1

Branch `chore/go-1.27`, **committato** il 29/09/2026: `bd9325c` (toolchain, dipendenze, Dockerfile, README) e `5838408` (`go fix`). Entrambi i commit sono stati verificati singolarmente (build, vet, test). Rotta Walden: manutenzione che preserva il contratto, quindi nessuna spec.

| Area | Modifica |
|---|---|
| `go.mod` di host e `plugins/hello-plugin` | `go 1.23.2` → `go 1.27.1`; il plugin deve seguire l'host |
| Dipendenze dirette | client_golang 1.20.4→1.24.1, client_model 0.6.1→0.6.3, testify 1.9.0→1.12.1, tint 1.0.5→1.2.0, color 1.16.0→1.19.0 |
| Dipendenze indirette | x/sys 0.22→0.48, protobuf 1.34.2→1.36.12, klauspost/compress 1.17.9→1.20.1, prometheus/common 0.55→0.72, procfs 0.15→0.22; testify non richiede più go-spew, go-difflib e kr/text |
| `go fix` (1.27) | `any`, range su interi, `slices.Contains`, `slices.Backward`, `maps.Copy`, `tint.NewHandler` → `NewTextHandler`: 42 modifiche su 9 file, tutte equivalenti |
| Dockerfile | Red Hat non ha un go-toolset 1.27 (massimo 1.26.7): `ubi8/go-toolset:1.26` come base + `GOTOOLCHAIN=go1.27.1` con `GOPROXY` e `GOSUMDB` espliciti; resta su UBI 8 per la glibc del runtime |
| README | requisito Go ≥ 1.27.1; i plugin vanno compilati con la stessa toolchain |

**Verifiche fatte**

- `go mod verify`, build, vet (host e plugin), test verdi.
- Test `-race`: solo i 3 fallimenti preesistenti.
- Prova completa in locale: firma, caricamento del plugin, middleware applicato.
- Immagine costruita con podman: binario e `.so` compilati con go1.27.1, plugin caricato nel container.
- govulncheck: da 31 a 0.

**Da fare**

- ~~Committare in due passi: toolchain e dipendenze, poi le modifiche di `go fix`.~~ Fatto.
- Aggiornare la toolchain locale: il GOENV ha `GOTOOLCHAIN=local` e il Go installato è 1.24.0.
- In CI e Sonar usare strumenti compilati con Go ≥ 1.27: un golangci-lint compilato con Go 1.25 rifiuta un modulo 1.27.

---

## 6. Registro delle evidenze

| ID | Grav. | Titolo | Evidenza | Dove | Spec |
|---|---|---|---|---|---|
| **A. Data path** | | | | | |
| F-01 | 🔴 | Risposte oltre il primo chunk troncate, con `Content-Length` coerente col troncamento | REPRO, E2E | `handlers/handlers.go:457-560`, `:136`; `config/config.go:178` | S-03 |
| F-02 | 🔴 | `r.ParseForm()` consuma il body dei form (502) e rifiuta le query malformate (400) | REPRO | `handlers/handlers.go:652` | S-04 |
| F-03 | 🟠 | Limite di 10 MB sul body della richiesta aggirabile con upload chunked | REPRO | `handlers/handlers.go:646` | S-04 |
| F-04 | 🟠 | Con `replace_path: false` la regex della location viene rimossa come se fosse un prefisso letterale | REPRO | `handlers/handlers.go:242` | S-07 |
| F-05 | 🟠 | `X-Forwarded-For` duplicato con porta; `X-Forwarded-Host` uguale all'host del backend | REPRO | `transport/transport.go:28-29,146-178`; `handlers/handlers.go:49-50,249` | S-07 |
| F-06 | 🟡 | Dettagli degli errori upstream restituiti al client (`upstream_error`) | REPRO | `handlers/handlers.go:297` | S-07 |
| F-07 | 🟡 | JSON della risposta 413 costruito con `Sprintf`, senza escape del path | CODE | `handlers/handlers.go:583` | S-03 |
| F-08 | 🟡 | Header di sicurezza sovrascritti su tutte le risposte (HSTS anche in HTTP) | CODE | `handlers/handlers.go:714` | S-09 |
| F-09 | 🟡 | `ReverseProxy` e `BufferPool` creati per ogni richiesta; `Director` deprecato da Go 1.26 | LINT, CODE | `handlers/handlers.go:213-219` | S-07 |
| F-10 | 🟡 | Tre wrapper di `ResponseWriter`, tee-buffer mai letto in produzione, nessun `Unwrap()` | CODE | `middlewares/logging.go:158`; `handlers/handlers.go:130-143`; `writer/writer.go` | S-03 |
| F-11 | 🟡 | Request ID da un generatore lineare seminato col tempo; `X-Request-ID` del client accettato senza validazione | CODE | `handlers/handlers.go:605-612,877-900` | S-07 |
| F-12 | 🔵 | `excluded_headers` confrontato con distinzione tra maiuscole e minuscole per gli `X-Forwarded-*` | CODE | `transport/transport.go:228` | S-07 |
| **B. Sicurezza** | | | | | |
| F-13 | 🔴 | Le route WebSocket non applicano i middleware dei plugin: bypass dell'autenticazione | REPRO | `handlers/handlers.go:98-101` | S-05 |
| F-14 | 🟠 | WebSocket: `CheckOrigin` sempre vero, nessun header inoltrato, nessun limite, path e TLS del target ignorati | CODE, LINT | `websocket/websocket.go:30,45,85` | S-05 |
| F-15 | 🟠 | Plugin fail-open: errori di caricamento solo loggati; blocco solo per middleware chiamati `auth` o `security` | CODE | `plugin/plugin.go:182`; `handlers/handlers.go:341` | S-08 |
| F-16 | 🟠 | `http.Server` senza timeout (Slowloris) | LINT | `cmd/main.go:126` | S-09 |
| F-17 | 🟠 | OpenShift: chiave privata montata nei pod, firma a runtime, `emptyDir` che nasconde i plugin dell'immagine | CODE | `deployments/openshift/production-deployment.yaml:33-35,50-53,76-79,106-107` | S-10 |
| F-18 | 🟡 | Panic se la chiave ed25519 ha lunghezza errata (`Verify`, `Sign`) | CODE | `plugin/plugin.go:69,165`; `cmd/plugin-signer/main.go:62` | S-08 |
| F-19 | 🟡 | TOCTOU tra la verifica della firma e `plugin.Open` | CODE | `plugin/plugin.go:52-69,119` | S-08 |
| F-20 | 🟡 | Dati sensibili nei log: header (`Authorization`, `Cookie`) e body in verbose, contenuto dei messaggi WebSocket | CODE | `logging/logging.go:79-90,127-160`; `websocket/websocket.go:90-97` | S-12 |
| F-21 | 🔵 | `net/http/pprof` registrato su `DefaultServeMux` (servito solo su localhost e con il flag) | LINT | `cmd/main.go:17,176` | S-09 |
| F-22 | 🔵 | Token Sonar in chiaro in `problems-bob.md`, file non tracciato ma neppure ignorato | OSS | `problems-bob.md` | M-01 |
| **C. Concorrenza e hot reload** | | | | | |
| F-23 | 🔴 | Data race su `Config` e `Logger` durante il reload; indice della location usato tra versioni diverse della config | RACE, CODE | `app/app.go:39-72`; `handlers/handlers.go:79-200`; `middlewares/logging.go:48,96,122`; `transport/transport.go:119` | S-06 |
| F-24 | 🟠 | Il ritorno alla config originale viene ignorato: la config globale non viene mai aggiornata (due fonti di verità) | REPRO | `cmd/main.go:53-58`; `config/config.go:327` | S-06 |
| F-25 | 🟡 | `TransportCache.Clear()` non chiude le connessioni idle; chiave della cache calcolata (JSON + SHA-256) a ogni richiesta; `genericTransport` inutilizzato | CODE | `transport/transport.go:51-115,233` | S-06 |
| F-26 | 🟡 | `WatchConfig` non cancellabile, attesa fissa di 1 s, errore loggato ogni 2 s se il file manca | CODE | `config/config.go:299-335` | S-06 |
| F-27 | 🟠 | `LimitedBuffer.Read` modifica il buffer tenendo solo `RLock`: race e panic | RACE | `writer/limited_buffer.go:78-88` | S-01 |
| **D. Osservabilità** | | | | | |
| F-28 | 🟠 | Ogni richiesta contata due volte (`http_requests_total`, durata, byte trasferiti) | REPRO | `middlewares/logging.go:122-125`; `handlers/handlers.go:792` | S-11 |
| F-29 | 🟡 | Label ad alta cardinalità (path), status in testo (`"OK"`), `active_connections` che conta le richieste | CODE, REPRO | `metrics/metrics.go:180-186` | S-11 |
| F-30 | 🟡 | `NormalizePath` compila 3 regex a ogni chiamata, 2 chiamate per richiesta | CODE | `metrics/metrics.go:163-176` | S-11 |
| F-31 | 🟡 | 11 metriche su 14 registrate ma mai valorizzate | CODE | `metrics/metrics.go` | S-11 |
| F-32 | 🟡 | `logging.enabled: false` non disattiva i log delle richieste; `verbose: true` con `level: info` non logga nulla | REPRO | `middlewares/logging.go:48-53`; `logging/logging.go:99` | S-12 |
| F-33 | 🟡 | Log solo testuali e colorati (niente JSON); logger ricreato al reload; goroutine di log avviate in `init()` che scartano log e li perdono allo shutdown | CODE | `logging/logging.go:32-63`; `middlewares/logging.go:28-45,140-155`; `app/app.go:68` | S-12 |
| F-34 | 🟡 | Nessun endpoint di health o readiness: probe e HEALTHCHECK su `/metrics` | CODE | `deployments/kubernetes/basic-deployment.yaml:47,53`; `Dockerfile:88` | S-09 |
| **E. Configurazione** | | | | | |
| F-35 | 🟠 | Dito non parte senza chiavi dei plugin (exit 1), anche senza plugin configurati | E2E | `plugin/plugin.go:147-160`; `cmd/main.go:83-91` | S-08 |
| F-36 | 🟡 | Chiavi YAML sconosciute ignorate (es. `idle_timeout` nel template k8s); URL, TLS e middleware non validati al caricamento | CODE | `config/config.go:142`; `configs/templates/application.yaml:34` | S-13 |
| F-37 | 🟡 | Config "morta": `rate_limiting`, `cache` ed `enable_compression` mai implementati | CODE | `config/config.go:117-119` | S-13 |
| F-38 | 🟡 | CA file non valido ignorato (`AppendCertsFromPEM` non controllato) | CODE | `transport/transport.go:205` | S-13 |
| F-39 | 🔵 | `gopkg.in/yaml.v3` archiviato; il successore `go.yaml.in/yaml/v3` è già tra le dipendenze indirette | OSS | `go.mod` | S-13 |
| **F. Qualità e tooling** | | | | | |
| F-40 | 🟠 | Nessuna CI | OSS | `.github/` | S-02 |
| F-41 | 🟡 | Test con race nel test stesso: `TestWatchConfig`, `TestConcurrentWrites` | RACE | `config/config_test.go:272`; `writer/writer_test.go:531` | S-01 |
| F-42 | 🟡 | Test non ermetico: `TestDynamicProxyHandler` dipende da `http://example.com` | CODE | `handlers/handlers_test.go:32,80` | S-01 |
| F-43 | 🟡 | Buchi di copertura: 0% su 4 package, transport 34%, handlers 47% | MISURA | §4.3 | S-02 |
| F-44 | 🔵 | Debito di lint: 105 issue, incluso codice morto | LINT | Appendice D | S-02, M-01 |
| F-45 | 🔵 | Makefile: `.PHONY` duplicato, niente target lint/race/vuln/cover, niente `-trimpath` né versione iniettata; flag di build di host e plugin da allineare | CODE | `Makefile:2,33` | S-02 |
| **G. Deploy e documentazione** | | | | | |
| F-46 | 🔵 | Documentazione ed esempi incoerenti: path `cmd/dito/main.go`, flag inesistenti del plugin-signer, hash hardcoded in `verify`, compose con context errato, tag immagine incoerenti, `ENV PORT` inutilizzata | CODE | `README.md:143,326,340,540`; `cmd/plugin-signer/main.go:13`; `deployments/docker/docker-compose.yml:1,6`; `Dockerfile:92` | M-01, S-08, S-10 |
| F-47 | 🔵 | 3 branch non mergiati e obsoleti (solo README, 2024) | OSS | git | M-01 |
| F-48 | 🔵 | Red Hat go-toolset in ritardo (massimo 1.26.7): build via `GOTOOLCHAIN`; FIPS da valutare | OSS | `Dockerfile` | S-10 |
| **H. Strategia** | | | | | |
| F-49 | 🟠 | Plugin basati sul package `plugin` di Go: CGO, stessa toolchain, dipendenze e flag tra host e plugin, niente unload né isolamento | OSS | `plugin/plugin.go`; `plugins/` | S-14 |
| **I. Emersi durante le spec** | | | | | |
| F-50 | 🔵 | `LimitedBuffer.Len()` e `Available()` non si aggiornano dopo `Read`: scritti e letti 11 byte, `Len()` resta 11 e `Available()` 89 (emerso durante S-01; `Read` non è usato in produzione) | REPRO | `writer/limited_buffer.go:78-110` | S-03 |

---

## 7. Dettaglio dei finding critici e alti

### F-01 🔴 Risposte troncate senza errore

- **Evidenza** (R-01, R-14): una risposta da 204.800 B arriva al client come 200 OK con `Content-Length: 3975` e 3.975 B (harness). Con il binario reale e un backend `python3 -m http.server` ne arrivano 16.332.
- **Causa**: al primo `Write`, `responseLimitInterceptor.Write` (`handlers.go:457`) chiama `flushBuffer()` (`:498-500`). Questo imposta `Content-Length` alla dimensione del primo chunk, scrive header e primo chunk e segna `headerWritten`. I `Write` successivi finiscono in `rli.buffer`, che non viene più svuotato: `Flush()` agisce solo se `!headerWritten`.
- **Perché colpisce tutte le location**: l'interceptor è attivo se il limite è > 0 (`:136`), e `validateAndSetDefaults` imposta 100 MB quando il limite non è configurato (`config.go:178`).
- **Impatto**:
  - corruzione silenziosa di ogni risposta più grande del primo chunk (tipicamente pochi KB);
  - il client non vede errori perché il `Content-Length` è coerente con i byte ricevuti;
  - il resto della risposta resta in memoria fino al limite, cioè fino a 100 MB per richiesta.
- **Direzione**: un solo wrapper che conta i byte e applica il limite senza bufferizzare; test con body di varie dimensioni e chunked.
- **Contratto**: ripristina quanto promesso dal README (sezione *Response Limits*).

### F-02 🔴 `ParseForm` consuma il body dei form

- **Evidenza**:
  - R-02: `POST` `application/x-www-form-urlencoded` con `a=1&b=2` → 502 con `net/http: HTTP/1.x transport connection broken: http: ContentLength=7 with Body length 0`;
  - R-03: `GET /q?a=%zz` → 400 "Invalid request format" restituito da Dito, senza contattare il backend.
- **Causa**: `validateRequest` chiama `r.ParseForm()` (`handlers.go:652`). Per i form, `ParseForm` legge tutto il body; sulle query non valide restituisce errore.
- **Impatto**: ogni POST, PUT o PATCH con form HTML o OAuth (per esempio i token endpoint) fallisce. Richieste che il backend accetterebbe vengono rifiutate.
- **Direzione**: il proxy non interpreta né body né query; `ParseForm` va rimosso.
- **Contratto**: ripristina (proxy trasparente).

### F-03 🟠 Limite sul body della richiesta aggirabile

- **Evidenza** (R-04): upload chunked di 11 MiB → 200, il backend riceve 11.534.336 B.
- **Causa**: il controllo usa solo `r.ContentLength` (`handlers.go:646`), che per le richieste chunked vale -1.
- **Impatto**: il limite di 10 MB non protegge né Dito né i backend.
- **Direzione**: `http.MaxBytesReader` sul body, limite configurabile (D-03), 413 quando viene superato.
- **Contratto**: ripristina e rende esplicito il limite.

### F-04 🟠 `replace_path: false` con location regex

- **Evidenza** (R-05): location `^/api` → `…/base`, richiesta `/api/users` → il backend riceve `/base/api/users` invece di `/base/users`.
- **Causa**: `strings.TrimPrefix(originalReq.URL.Path, location.Path)` (`handlers.go:242`) usa il pattern (`^/api`) come stringa letterale, che non combacia mai se contiene metacaratteri.
- **Impatto**: routing errato per ogni location non letterale con `replace_path: false`.
- **Direzione**: definire la semantica (D-06) e usare il match della regex compilata.
- **Contratto**: cambia; oggi la semantica non è definita.

### F-05 🟠 Header `X-Forwarded-*` errati

- **Evidenza** (R-06): con `Host: public.example.com`, il backend riceve `X-Forwarded-For: 127.0.0.1, 127.0.0.1:50476` e un `X-Forwarded-Host` uguale al proprio host.
- **Cause**:
  1. In modalità `Director`, `ReverseProxy` aggiunge già il client a `X-Forwarded-For`; poi `Caronte.AddHeaders` (`transport.go:146-153`) aggiunge di nuovo `RemoteAddr`, porta compresa.
  2. `handlers` salva host e protocollo originali con chiavi di tipo `handlers.contextKey` (`handlers.go:49-50`), mentre `transport` le cerca con `transport.contextKey` (`transport.go:28-29`). I tipi diversi rendono il lookup sempre vuoto, e il fallback su `req.Host` legge l'host già riscritto dal Director (`handlers.go:249`).
  3. `X-Forwarded-Proto` ripiega sul valore inviato dal client, che è falsificabile.
- **Impatto**: i backend che costruiscono URL assoluti, redirect o regole CORS, o che fanno controlli di sicurezza su host e IP, ricevono dati sbagliati. Gli IP con porta rompono i parser.
- **Direzione**: `ReverseProxy.Rewrite` con `pr.SetXForwarded()`, policy sui proxy fidati (D-07).
- **Contratto**: cambia; va definita la policy.

### F-13 🔴 WebSocket senza middleware

- **Evidenza** (R-07): location con `enable_websocket: true` e `middlewares: [auth]`, dove `auth` nega sempre. Una GET normale riceve 401; l'upgrade WebSocket si connette e il backend risponde all'echo.
- **Causa**: il ramo WebSocket (`handlers.go:98-101`) chiama `HandleWebSocketProxy` e ritorna prima di `handleLocationMatch`, l'unico punto in cui si applicano i middleware.
- **Impatto**: autenticazione e ogni altro controllo dei plugin sono aggirabili su qualunque route con WebSocket attivo.
- **Direzione**: applicare la catena di middleware prima dell'upgrade; a regime passare all'upgrade nativo di `ReverseProxy` (D-04).
- **Contratto**: cambia; i middleware diventano obbligatori anche per il WebSocket.

### F-14 🟠 Proxy WebSocket incompleto

- **Evidenza**: codice; il lint segnala anche confronti sempre veri in `websocket.go` (SA4023).
- **Cause**:
  - `CheckOrigin` restituisce sempre `true` (`websocket.go:30`);
  - `DefaultDialer.Dial(url, nil)` (`:45`) non inoltra header (Cookie, Authorization, Origin, sottoprotocolli) e non usa la config TLS/transport della location;
  - `ReadMessage` senza `SetReadLimit` (`:85`);
  - path e query della richiesta ignorati;
  - messaggi loggati a Info con il contenuto.
- **Impatto**: Cross-Site WebSocket Hijacking; backend autenticati inutilizzabili; DoS di memoria con messaggi enormi; mTLS non applicabile.
- **Direzione**: come F-13.
- **Contratto**: cambia.

### F-15 🟠 Plugin fail-open

- **Evidenza**: codice.
- **Causa**:
  - un plugin che non si carica (firma, `plugin.Open`, simbolo mancante) viene solo loggato (`plugin.go:182`);
  - un middleware referenziato ma assente blocca le richieste solo se si chiama `auth` o `security` (`handlers.go:341`); altrimenti produce un warning a ogni richiesta.
- **Impatto**: se un plugin di autenticazione con un nome qualunque (es. `jwt-auth`) non si carica, la route resta esposta senza protezione.
- **Direzione**: validazione all'avvio e al reload (D-08).
- **Contratto**: cambia.

### F-16 🟠 Server senza timeout

- **Evidenza**: gosec G112.
- **Causa**: `&http.Server{Addr, Handler}` (`cmd/main.go:126`) senza `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` né `IdleTimeout`.
- **Impatto**: con attacchi Slowloris o connessioni appese si esauriscono file descriptor e goroutine.
- **Direzione**: timeout configurabili con default sicuri, compatibili con streaming e WebSocket.
- **Contratto**: nuovo.

### F-17 🟠 OpenShift: firma a runtime e chiave privata nei pod

- **Evidenza**: lettura dei manifest, non eseguiti su un cluster.
- **Cause**:
  - l'init container copia `/app/keys/ed25519_private.key` e firma i plugin all'avvio (`production-deployment.yaml:33-35`);
  - il secret con la chiave privata è montato anche nel container principale (`:76`);
  - l'`emptyDir` `plugins-shared` montato su `/app/plugins` (`:52-53`, `:78-79`, `:106-107`) nasconde i plugin dell'immagine, quindi quasi certamente nessun plugin viene firmato né caricato;
  - il README presenta la firma a runtime come funzionalità di sicurezza (`README.md:540`).
- **Impatto**: chi compromette il pod può firmare plugin arbitrari, e la firma perde valore. In più il deploy "production" gira senza plugin.
- **Direzione**: firma in CI, nei pod solo la chiave pubblica, plugin firmati dentro l'immagine (D-11).
- **Contratto**: cambia.

### F-23 🔴 Data race durante l'hot reload

- **Evidenza** (R-09): con 4 client in loop e 20 reload, il race detector produce 34 report. Le letture sono in `DynamicProxyHandler`, `handleLocationMatch`, `createProxyHandler`, `ServeProxy`, `getTimeout`, `processLogEntry`, `Caronte.RoundTrip` e `GetTransport`; le scritture concorrenti in `UpdateConfig` e `UpdateComponents`.
- **Causa**:
  - `UpdateConfig` e `UpdateComponents` scrivono `Config` e `Logger` sotto lock (`app.go:39-72`), ma tutti i lettori accedono a `dito.Config` e `dito.Logger` senza lock;
  - `DynamicProxyHandler` trova l'indice della location, poi `handleLocationMatch` e `ServeProxy` rileggono `dito.Config.Locations[i]`. Se nel frattempo arriva un reload, la richiesta può finire sulla location sbagliata (backend e middleware diversi) o andare in panic per indice fuori range.
- **Impatto**: comportamento indefinito sotto carico durante i reload; possibile instradamento verso un backend con middleware diversi, quindi rischio di sicurezza.
- **Direzione**: snapshot immutabile della config per ogni richiesta (`atomic.Pointer`) e passaggio della location invece dell'indice (D-05).
- **Contratto**: ripristina l'hot reload "senza downtime" promesso dal README.

### F-24 🟠 Il revert della config viene ignorato

- **Evidenza** (R-10): il file passa da 1111 a 2222 e poi di nuovo a 1111; `onChange` viene chiamato solo per 2222, e Dito resta su 2222.
- **Causa**: `onChange` (`cmd/main.go:53-58`) aggiorna solo l'istanza `Dito`, mai la config globale. `WatchConfig` confronta la nuova config con quella globale (`config.go:327`), che resta quella di avvio.
- **Impatto**: un rollback, cioè il caso più comune durante un incidente, non viene applicato.
- **Direzione**: un'unica fonte di verità per la config corrente.
- **Contratto**: ripristina.

### F-27 🟠 Race in `LimitedBuffer.Read`

- **Evidenza**: `TestLimitedBuffer_ConcurrentAccess` con `-race` produce una race e `panic: slice bounds out of range`.
- **Causa**: `Read` e `String` prendono solo `RLock` (`limited_buffer.go:78-88`), ma `bytes.Buffer.Read` modifica lo stato interno del buffer.
- **Impatto**: panic in caso di letture concorrenti; oggi impedisce di usare `-race` come gate.
- **Direzione**: lock esclusivo in `Read`, oppure eliminare il tipo se S-03 rimuove i tee-buffer.
- **Contratto**: ripristina.

### F-28 🟠 Metriche contate due volte

- **Evidenza** (R-08): dopo 1 GET e 1 POST, `http_requests_total` vale 2 per ciascuno.
- **Causa**: `RecordRequest` e `RecordDataTransferred` vengono chiamati sia da `LoggingMiddleware` (`middlewares/logging.go:122-125`) sia da `recordMetrics` (`handlers.go:792`).
- **Impatto**: traffico, latenze e byte sovrastimati; alert e capacity planning falsati.
- **Direzione**: un solo punto di registrazione (S-11, dopo S-03).
- **Contratto**: ripristina.

### F-35 🟠 Dito non parte senza plugin

- **Evidenza** (R-13): con una config senza sezione `plugins` il binario stampa `Error loading plugins ... failed to read public key: open : no such file or directory` ed esce con codice 1.
- **Causa**: `LoadAndVerifyPlugins` (`plugin.go:147-160`) viene eseguito sempre e valida la chiave anche con `plugins.directory` vuoto, mentre la validazione della config lo consente.
- **Impatto**: impossibile usare Dito come semplice reverse proxy; la configurazione minima fallisce.
- **Direzione**: plugin opzionali (D-09).
- **Contratto**: cambia.

### F-40 🟠 Nessuna CI

- **Evidenza**: `.github/` contiene solo `FUNDING.yml`.
- **Impatto**: nessuno dei bug di questo documento è stato intercettato; le race nei test sono passate inosservate.
- **Direzione**: build, vet, `test -race`, golangci-lint v2, govulncheck, build dell'immagine, smoke end-to-end dei plugin, aggiornamento automatico delle dipendenze.
- **Contratto**: nuovo.

### F-49 🟠 Modello di plugin basato sul package `plugin` di Go

- **Evidenza**: durante l'upgrade il `go.mod` del plugin ha dovuto seguire quello dell'host.
- **Vincoli**:
  - CGO obbligatorio: niente binari statici né immagini distroless;
  - solo Linux, macOS e FreeBSD;
  - host e plugin devono avere la stessa toolchain, le stesse versioni di tutte le dipendenze condivise e gli stessi flag di build (`-trimpath`, tag);
  - nessun unload;
  - nessun isolamento: un plugin può fare tutto quello che fa Dito, e la firma ne garantisce solo la provenienza.
- **Impatto**: ogni upgrade di Go o di una dipendenza condivisa rompe i plugin di terze parti.
- **Direzione**: ADR (D-15) tra registry a compile-time (stile Caddy), WASM (wazero) e plugin fuori processo (gRPC).
- **Contratto**: cambia.

---

## 8. Portfolio delle spec Walden

### 8.1 Ondate

| Ondata | Obiettivo | Spec |
|---|---|---|
| 0 — Fondamenta | proof affidabili e gate automatici | baseline Go (fatta), S-01, S-02 |
| 1 — P0 | eliminare i 🔴 | S-03, S-04, S-06, S-05 |
| 2 — P1 | eliminare i 🟠 rimanenti | S-07, S-08, S-09, S-10 |
| 3 — P2 | osservabilità e configurazione | S-11, S-12, S-13 |
| 4 — Strategia | modello di estensione | S-14 |

### 8.2 Dipendenze

```
S-01 test-suite-hygiene        ──► tutte (le proof usano go test -race)
S-02 ci-quality-gates          ──► in parallelo a S-01; il gate -race si attiva dopo S-01
S-03 response-body-integrity   ──► S-05 (Hijack/Unwrap per l'upgrade nativo), S-11 (un solo writer da misurare)
S-06 hot-reload-consistency    ──► S-07 (proxy per location costruiti sullo snapshot)
S-08 plugin-loading-hardening  ──► S-10 (flusso di firma), S-13 (validazione dei middleware referenziati)
S-09 server-hardening          ──► S-10 (probe su /healthz e /readyz)
S-14 plugin-architecture       ──  l'ADR può partire subito; l'implementazione dopo l'ondata 2
```

Nota su S-05: il primo task può chiudere il bypass (F-13) subito, senza aspettare S-03.

### 8.3 Schede delle spec

I "temi di accettazione" sono il materiale di partenza per i requirements EARS: non sono ancora criteri formali.

#### S-01 `test-suite-hygiene` — ondata 0 · contratto: ripristina

- **Obiettivo**: `go test -race -count=1 ./...` verde, ermetico e deterministico.
- **Finding**: F-27, F-41, F-42.
- **Temi di accettazione**:
  - nessuna race nei package di produzione;
  - nessun test contatta la rete esterna;
  - i test concorrenti rispettano il contratto di `http.ResponseWriter`;
  - `TestWatchConfig` deterministico e senza attese di secondi (es. `testing/synctest`).
- **Fuori scope**: nuova copertura funzionale, che arriva con le spec di area.
- **Dipendenze**: nessuna.

#### S-02 `ci-quality-gates` — ondata 0 · contratto: nuovo

- **Obiettivo**: ogni PR verificata automaticamente.
- **Finding**: F-40, F-43, F-44, F-45.
- **Temi di accettazione**:
  - workflow con build, vet, `test -race`, golangci-lint v2 con configurazione versionata (baseline che blocca solo le issue nuove), govulncheck bloccante, build dell'immagine;
  - smoke end-to-end: build di host e plugin, firma, caricamento, richiesta proxata;
  - target Makefile equivalenti;
  - aggiornamento automatico di dipendenze e immagini base;
  - strumenti fissati con direttive `tool` in `go.mod`;
  - soglie di copertura (valori da decidere, vedi K-5).
- **Fuori scope**: pubblicazione automatica delle release.
- **Dipendenze**: il gate `-race` richiede S-01.

#### S-03 `response-body-integrity` — ondata 1 · contratto: ripristina (+ D-01, D-02)

- **Obiettivo**: risposte integre e limiti di dimensione affidabili.
- **Finding**: F-01, F-07, F-10, F-50.
- **Temi di accettazione**:
  - le risposte sotto il limite arrivano integre (byte e `Content-Length`) per ogni dimensione e chunking;
  - un `Content-Length` dichiarato oltre il limite produce un errore strutturato prima del body;
  - il superamento durante lo streaming è gestito secondo D-01;
  - il body di errore è JSON valido per qualunque path;
  - la memoria per richiesta non cresce con la dimensione della risposta;
  - `Flush`, `Hijack` e `http.ResponseController` sono raggiungibili attraverso il wrapper.
- **Fuori scope**: compressione, caching.
- **Direzione tecnica**: un solo wrapper (status, byte, limite) con `Unwrap()`; rimozione di `responseLimitInterceptor` e dei tee-buffer.
- **Dipendenze**: S-01. Abilita S-05 e S-11.

#### S-04 `request-body-forwarding` — ondata 1 · contratto: ripristina (+ D-03)

- **Obiettivo**: body e query della richiesta arrivano al backend invariati.
- **Finding**: F-02, F-03.
- **Temi di accettazione**:
  - form urlencoded e multipart inoltrati byte per byte;
  - query non valide inoltrate senza intervento del proxy;
  - body oltre il limite, anche chunked, → 413;
  - limite configurabile.
- **Dipendenze**: S-01.

#### S-05 `websocket-proxy-security` — ondata 1 · contratto: cambia (+ D-04)

- **Obiettivo**: il WebSocket ha le stesse garanzie delle route HTTP.
- **Finding**: F-13, F-14 e la parte WebSocket di F-20.
- **Temi di accettazione**:
  - i middleware della location valgono anche per l'upgrade;
  - policy sull'origin;
  - header e sottoprotocolli inoltrati;
  - limite sulla dimensione dei messaggi;
  - TLS e transport della location applicati;
  - path e query rispettati;
  - contenuto dei messaggi non loggato.
- **Direzione tecnica**: upgrade nativo di `ReverseProxy`, che toglie gorilla dal data path.
- **Dipendenze**: S-01; S-03 per la migrazione. Il fix del bypass può precedere S-03.

#### S-06 `hot-reload-consistency` — ondata 1 · contratto: ripristina (+ D-05)

- **Obiettivo**: reload atomici e privi di race.
- **Finding**: F-23, F-24, F-25, F-26.
- **Temi di accettazione**:
  - ogni richiesta usa una sola versione della config dall'inizio alla fine;
  - nessuna race con reload concorrenti;
  - il revert alla config precedente viene applicato;
  - una config non valida fa rifiutare il reload e resta attiva quella corrente;
  - le connessioni dei transport rimossi vengono chiuse;
  - il watcher si ferma allo shutdown;
  - il livello di log cambia senza sostituire il logger.
- **Direzione tecnica**: `atomic.Pointer[ProxyConfig]` come unica fonte di verità; lo stato derivato (regex, transport, proxy) si costruisce al caricamento.
- **Dipendenze**: S-01. Abilita S-07.

#### S-07 `upstream-routing-and-headers` — ondata 2 · contratto: cambia (+ D-06, D-07)

- **Obiettivo**: la richiesta verso il backend è costruita in modo corretto e prevedibile.
- **Finding**: F-04, F-05, F-06, F-09, F-11, F-12.
- **Temi di accettazione**:
  - riscrittura del path secondo D-06;
  - `X-Forwarded-For`, `X-Forwarded-Host` e `X-Forwarded-Proto` corretti secondo D-07;
  - errori upstream senza dettagli interni;
  - request ID non prevedibili (es. `uuid.NewV7()` della libreria standard) e validazione di quelli in ingresso;
  - `excluded_headers` senza distinzione tra maiuscole e minuscole;
  - un `ReverseProxy` per location con buffer pool condiviso.
- **Direzione tecnica**: `ReverseProxy.Rewrite` al posto di `Director`.
- **Dipendenze**: S-06.

#### S-08 `plugin-loading-hardening` — ondata 2 · contratto: cambia (+ D-08, D-09)

- **Obiettivo**: il caricamento dei plugin è sicuro e, se fallisce, blocca invece di proseguire.
- **Finding**: F-15, F-18, F-19, F-35 e la parte plugin-signer di F-46.
- **Temi di accettazione**:
  - un middleware referenziato ma non caricato fa rifiutare avvio o reload;
  - plugin opzionali se non configurati;
  - chiavi di formato o lunghezza errati producono un errore, non un panic;
  - il file verificato è lo stesso che viene aperto;
  - `plugin-signer` con percorsi e flag come da documentazione e senza hash hardcoded.
- **Dipendenze**: S-01. Abilita S-10 e S-13.

#### S-09 `server-hardening` — ondata 2 · contratto: nuovo/cambia (+ D-10)

- **Obiettivo**: il listener regge client lenti o ostili e si può sondare.
- **Finding**: F-08, F-16, F-21, F-34.
- **Temi di accettazione**:
  - timeout del server configurabili, con default sicuri e compatibili con streaming e WebSocket;
  - `/healthz` (liveness) e `/readyz` (config e plugin caricati);
  - pprof solo su un listener dedicato;
  - policy degli header di sicurezza secondo D-10;
  - timeout di shutdown configurabile.
- **Dipendenze**: S-01. Abilita S-10.

#### S-10 `deployment-supply-chain` — ondata 2 · contratto: cambia (+ D-11)

- **Obiettivo**: artefatti riproducibili e firmati fuori dal runtime.
- **Finding**: F-17, F-48 e la parte immagini/compose di F-46.
- **Temi di accettazione**:
  - plugin firmati in CI e inclusi nell'immagine;
  - nessuna chiave privata nei pod;
  - manifest k8s e OpenShift coerenti: probe sugli endpoint di health, niente `emptyDir` sui plugin, tag versionati;
  - toolchain dell'immagine fissata e verificata;
  - versione iniettata nel binario;
  - requisiti FIPS valutati.
- **Dipendenze**: S-08, S-09.

#### S-11 `metrics-accuracy` — ondata 3 · contratto: cambia (+ D-12)

- **Obiettivo**: metriche esatte e con cardinalità limitata.
- **Finding**: F-28, F-29, F-30, F-31.
- **Temi di accettazione**:
  - una sola registrazione per richiesta;
  - label per location e status numerico;
  - nessuna regex compilata per richiesta;
  - ogni metrica esposta viene alimentata, oppure rimossa;
  - il gauge delle richieste in corso ha il nome giusto.
- **Dipendenze**: S-03.

#### S-12 `structured-logging` — ondata 3 · contratto: cambia (+ D-13)

- **Obiettivo**: log utili, sicuri e coerenti con la configurazione.
- **Finding**: F-20, F-32, F-33.
- **Temi di accettazione**:
  - `logging.enabled`, `verbose` e `level` hanno l'effetto documentato;
  - formato JSON per l'aggregazione dei log;
  - header e body sensibili redatti;
  - nessuna goroutine avviata in `init()`;
  - nessuna perdita silenziosa di log allo shutdown.
- **Dipendenze**: S-06, per aggiornare il livello al reload.

#### S-13 `config-validation` — ondata 3 · contratto: cambia (+ D-14)

- **Obiettivo**: config sbagliate rifiutate al caricamento invece che scoperte a runtime.
- **Finding**: F-36, F-37, F-38, F-39.
- **Temi di accettazione**:
  - chiavi sconosciute segnalate;
  - URL target, file TLS e CA, regex e middleware validati al caricamento;
  - campi non implementati rifiutati o implementati secondo D-14;
  - migrazione a `go.yaml.in/yaml/v3`, che richiede l'approvazione del cambio di dipendenza.
- **Dipendenze**: S-08.

#### S-14 `plugin-architecture` — ondata 4 · contratto: cambia (+ D-15)

- **Obiettivo**: un modello di estensione che sopravviva agli upgrade di Go.
- **Finding**: F-49.
- **Temi di accettazione**:
  - ADR con confronto tra le opzioni;
  - compatibilità o migrazione per `hello-plugin`;
  - binari statici possibili (`CGO_ENABLED=0`);
  - isolamento dei plugin se si sceglie WASM o fuori processo.
- **Dipendenze**: l'ADR può partire subito; l'implementazione dopo l'ondata 2.

#### M-01 Manutenzione fuori Walden

- F-22: redigere o ignorare `problems-bob.md` e ruotare il token se il file è stato condiviso; aggiungere a `.gitignore` `coverage.out` e i backup di Sonar.
- F-46: correzioni di README ed esempi.
- F-47: eliminazione dei branch obsoleti, solo dopo conferma.
- Parte residua di F-44: pulizia del lint che non cambia comportamento, verificata dalla CI.

Per Walden sono manutenzioni che preservano il contratto: si verificano con i test esistenti senza aprire una spec, a meno che non si decida diversamente.

### 8.4 Tracking

| Spec | Nome | Ondata | Finding | Stato | Spec Walden |
|---|---|---|---|---|---|
| — | baseline Go 1.27.1 | 0 | — | fatta, committata (`bd9325c`, `5838408`) | — (manutenzione) |
| S-01 | `test-suite-hygiene` | 0 | F-27, F-41, F-42 | completata il 29/09/2026: 6/6 task `verified`, da committare | `.walden/specs/test-suite-hygiene/` |
| S-02 | `ci-quality-gates` | 0 | F-40, F-43, F-44, F-45 | da iniziare | — |
| S-03 | `response-body-integrity` | 1 | F-01, F-07, F-10, F-50 | da iniziare | — |
| S-04 | `request-body-forwarding` | 1 | F-02, F-03 | da iniziare | — |
| S-05 | `websocket-proxy-security` | 1 | F-13, F-14 | da iniziare | — |
| S-06 | `hot-reload-consistency` | 1 | F-23, F-24, F-25, F-26 | da iniziare | — |
| S-07 | `upstream-routing-and-headers` | 2 | F-04, F-05, F-06, F-09, F-11, F-12 | da iniziare | — |
| S-08 | `plugin-loading-hardening` | 2 | F-15, F-18, F-19, F-35 | da iniziare | — |
| S-09 | `server-hardening` | 2 | F-08, F-16, F-21, F-34 | da iniziare | — |
| S-10 | `deployment-supply-chain` | 2 | F-17, F-48 | da iniziare | — |
| S-11 | `metrics-accuracy` | 3 | F-28, F-29, F-30, F-31 | da iniziare | — |
| S-12 | `structured-logging` | 3 | F-20, F-32, F-33 | da iniziare | — |
| S-13 | `config-validation` | 3 | F-36, F-37, F-38, F-39 | da iniziare | — |
| S-14 | `plugin-architecture` | 4 | F-49 | da iniziare | — |
| M-01 | manutenzione | — | F-22, F-44 (residuo), F-46, F-47 | da iniziare | — |

---

## 9. Decisioni aperte

Le raccomandazioni sono proposte da confermare: ogni decisione si chiude nei requirements o nel design della spec corrispondente e viene annotata qui.

| ID | Spec | Domanda | Opzioni | Raccomandazione |
|---|---|---|---|---|
| D-01 | S-03 | Risposta oltre il limite: status e comportamento durante lo streaming | status 413 (documentato) o 502; in streaming troncare o chiudere la connessione | 502, perché 413 si riferisce alla richiesta; in streaming chiudere la connessione, così il client vede un errore invece di un body troncato che sembra valido. Cambio breaking accettabile in 0.x |
| D-02 | S-03 | Tee-buffer dei body | mantenerlo per il debug; rimuoverlo | rimuoverlo: nessun consumer in produzione |
| D-03 | S-04 | Limite sul body della richiesta | fisso a 10 MB; configurabile | configurabile (globale e per location), default 10 MB |
| D-04 | S-05 | Implementazione del proxy WebSocket e policy sull'origin | gorilla corretto; upgrade nativo di `ReverseProxy`. Origin: inoltrato al backend o allowlist | upgrade nativo; origin inoltrato al backend con allowlist opzionale in config |
| D-05 | S-06 | Semantica del reload | snapshot per richiesta; config sempre aggiornata | snapshot per richiesta; i WebSocket aperti restano sulla config con cui sono nati |
| D-06 | S-07 | Significato di `replace_path: false` con location regex | togliere il match iniziale; nuovo campo `strip_prefix` letterale; nessuna rimozione | togliere il match della regex quando parte dall'inizio del path |
| D-07 | S-07 | Fiducia negli `X-Forwarded-*` in ingresso | sempre; mai; solo da `trusted_proxies` | sovrascrivere di default; preservare la catena solo per i `trusted_proxies` configurati |
| D-08 | S-08 | Middleware referenziato ma non disponibile | rifiutare avvio e reload; 503 sulla sola route | rifiutare avvio e reload; al reload resta attiva la config precedente |
| D-09 | S-08 | Plugin senza `plugins.directory` | obbligatori; opzionali | opzionali |
| D-10 | S-09 | Header di sicurezza aggiunti alle risposte | rimuoverli; renderli configurabili; aggiungerli solo se assenti | aggiungerli solo se il backend non li imposta, HSTS solo su TLS, disattivabili per location |
| D-11 | S-10 | Dove vive la chiave privata di firma | secret della CI; KMS; firma keyless (cosign) | secret della CI subito; cosign come evoluzione |
| D-12 | S-11 | Label e nomi delle metriche | pattern regex; nuovo campo `name` per location; prefisso `dito_` | campo `name` (default: il pattern), status numerico, prefisso `dito_`; breaking per le dashboard |
| D-13 | S-12 | Formato dei log | testo colorato; JSON; configurabile | `logging.format` (`json` o `text`), default `json` fuori da un terminale; redazione di `Authorization`, `Proxy-Authorization`, `Cookie` e `Set-Cookie` più una lista configurabile |
| D-14 | S-13 | Chiavi sconosciute e config non implementata | errore; warning con periodo di deprecazione; implementare le funzionalità | errore in 0.x; i campi non implementati vengono rifiutati finché non esistono |
| D-15 | S-14 | Modello di plugin | registry a compile-time; WASM (wazero); fuori processo (gRPC); status quo | da decidere con un ADR. Orientamento: registry nel breve periodo, WASM se i plugin di terze parti sono un obiettivo |

---

## 10. Criteri di successo

| ID | Criterio | Misura |
|---|---|---|
| K-1 | Nessun finding 🔴 o 🟠 aperto | tracking in §8.4 |
| K-2 | Test ermetici e senza race | `go test -race -count=1 ./...` verde in CI senza accesso alla rete |
| K-3 | Nessuna vulnerabilità raggiungibile | govulncheck bloccante in CI |
| K-4 | Nessun nuovo debito di lint | golangci-lint v2 blocca le issue nuove; la baseline cala |
| K-5 | Copertura | ≥ 70% totale e ≥ 50% per ogni package di produzione (valori da confermare) |
| K-6 | Plugin verificati end-to-end | smoke in CI: build di host e plugin, firma, caricamento, richiesta proxata |
| K-7 | Conformità del proxy | scenari R-01…R-14 trasformati in test di regressione verdi |

---

## 11. Governance e bootstrap Walden

**Prerequisiti**

1. Committare la baseline Go 1.27.1 e partire da un working tree pulito: Walden lega evidenze e approvazioni allo stato git.
2. CLI: nel `PATH` c'è `walden` 0.10.4 (`~/go/bin`), in `~/.local/bin` c'è la 0.11.0. Sono entrambe compatibili, ma conviene usarne una sola per tutto il portfolio.
3. `walden repo init`, poi compilare `.walden/constitution.md` con il contesto qui sotto.

**Stato del bootstrap (29/09/2026)**: baseline committata; `walden repo init` eseguito con la CLI 0.11.0; constitution compilata con il contesto approvato; S-01 completata (6/6 task con evidenze `verified`).

**Contesto per la constitution** (approvato il 29/09/2026 e applicato in `.walden/constitution.md`)

- **Stack**: Go 1.27.1, modulo `dito`; binari `cmd/` (proxy) e `cmd/plugin-signer`; plugin tramite il package `plugin` (CGO), compilati con la stessa toolchain dell'host.
- **Verifiche standard**: `go vet ./...`, `go test -race -count=1 ./...`, golangci-lint v2 compilato con Go ≥ 1.27, `govulncheck ./...`.
- **Regole proposte**:
  - test ermetici (`httptest`, niente rete esterna);
  - spec di bugfix in TDD: il test di regressione fallisce prima del fix;
  - nessuna nuova dipendenza senza approvazione esplicita;
  - ogni cambio di comportamento o di config aggiorna il README e include note di migrazione;
  - default di sicurezza fail-closed;
  - un cambio all'interfaccia `plugin.Plugin` è un cambio di contratto;
  - commit solo su richiesta esplicita.
- **Lingua**: descrizioni in italiano, parole chiave EARS e identificatori in inglese (da confermare).

**Regole di processo**

- Una spec per volta nelle ondate 0 e 1; dall'ondata 2 si può parallelizzare dove non ci sono dipendenze.
- Ogni requirements cita i finding di origine. Se una spec rivela un problema nuovo, lo si aggiunge qui con un nuovo ID.
- Le decisioni `D-xx` si chiudono nei requirements o nel design della spec corrispondente e si annotano in §9.

---

## Appendice A — Log delle verifiche

Tutte eseguite il 29/09/2026.

| # | Verifica | Metodo | Esito |
|---|---|---|---|
| A-1 | Build e vet (1.24.0 e 1.27.1) | `go build ./...`, `go vet ./...` | OK |
| A-2 | Test | `go test -count=1 -cover ./...` | tutti verdi |
| A-3 | Race detector | `go test -race -count=1 ./...` | 3 test falliti (config, writer) |
| A-4 | Vulnerabilità prima dell'upgrade | `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` con Go 1.24.0 | 31 raggiungibili nella stdlib (Appendice B) |
| A-5 | Vulnerabilità dopo l'upgrade | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` con Go 1.27.1 | nessuna |
| A-6 | Aggiornamenti disponibili | `go list -m -u all` | vedi §5 |
| A-7 | Modernizzazione | `go fix -diff ./...` | 42 modifiche su 9 file, applicate |
| A-8 | Lint | golangci-lint 2.14.0 (Appendice D) | 105 issue |
| A-9 | Harness di riproduzione | test temporanei (Appendice C) | R-01…R-12 |
| A-10 | E2E in locale | binario + `plugin-signer` + `hello-plugin.so` firmato + backend `python3 -m http.server` | plugin OK; `/big` troncata (R-14) |
| A-11 | Avvio senza plugin | binario con config minima | exit 1 (R-13) |
| A-12 | Immagine container | `podman build --platform linux/amd64`; `go version` sui binari estratti; avvio nel container con firma e richiesta | binari go1.27.1; plugin caricato; header `X-Hello-Plugin` presente |
| A-13 | Disponibilità della toolchain | API del registry Red Hat e di Docker Hub | go-toolset al massimo 1.26.7 (ubi8, ubi9, ubi10); `golang:1.27.1` disponibile |

---

## Appendice B — Vulnerabilità con la toolchain 1.24.0

Tutte nella libreria standard e raggiungibili dal codice di Dito. Nessuna è presente con Go 1.27.1.

| ID | Package | Corretta in | Descrizione |
|---|---|---|---|
| GO-2026-6090 | crypto/tls | go1.25.13 | Limit handshake messages we are willing to accept post-handshake |
| GO-2026-6089 | net/http | go1.25.13 | Apply ReadHeaderTimeout when doing unencrypted HTTP/2 check |
| GO-2026-5972 | encoding/asn1 | go1.25.13 | Enforce maximum recursion depth |
| GO-2026-5856 | crypto/tls | go1.25.12 | Encrypted Client Hello privacy leak |
| GO-2026-5039 | net/textproto | go1.25.11 | Arbitrary inputs are included in errors without any escaping |
| GO-2026-5037 | crypto/x509 | go1.25.11 | Inefficient candidate hostname parsing |
| GO-2026-5026 | net/http | go1.25.13 | Failure to reject ASCII-only Punycode-encoded labels (idna) |
| GO-2026-4976 | net/http/httputil | go1.25.10 | ReverseProxy forwards queries with more than urlmaxqueryparams parameters |
| GO-2026-4971 | net | go1.25.10 | Panic in Dial and LookupPort when handling NUL byte on Windows |
| GO-2026-4947 | crypto/x509 | go1.25.9 | Unexpected work during chain building |
| GO-2026-4946 | crypto/x509 | go1.25.9 | Inefficient policy validation |
| GO-2026-4918 | net/http | go1.25.10 | Infinite loop in HTTP/2 transport with bad SETTINGS_MAX_FRAME_SIZE |
| GO-2026-4870 | crypto/tls | go1.25.9 | Unauthenticated TLS 1.3 KeyUpdate record can cause connection retention and DoS |
| GO-2026-4602 | os | go1.25.8 | FileInfo can escape from a Root |
| GO-2026-4601 | net/url | go1.25.8 | Incorrect parsing of IPv6 host literals |
| GO-2026-4341 | net/url | go1.24.12 | Memory exhaustion in query parameter parsing |
| GO-2026-4340 | crypto/tls | go1.24.12 | Handshake messages may be processed at the incorrect encryption level |
| GO-2026-4337 | crypto/tls | go1.24.13 | Unexpected session resumption |
| GO-2025-4175 | crypto/x509 | go1.24.11 | Improper application of excluded DNS name constraints for wildcard names |
| GO-2025-4155 | crypto/x509 | go1.24.11 | Excessive resource consumption when printing host validation errors |
| GO-2025-4013 | crypto/x509 | go1.24.8 | Panic when validating certificates with DSA public keys |
| GO-2025-4012 | net/http | go1.24.8 | Lack of limit when parsing cookies can cause memory exhaustion |
| GO-2025-4011 | encoding/asn1 | go1.24.8 | Parsing DER payload can cause memory exhaustion |
| GO-2025-4010 | net/url | go1.24.8 | Insufficient validation of bracketed IPv6 hostnames |
| GO-2025-4009 | encoding/pem | go1.24.8 | Quadratic complexity when parsing some invalid inputs |
| GO-2025-4008 | crypto/tls | go1.24.8 | ALPN negotiation error contains attacker-controlled information |
| GO-2025-4007 | crypto/x509 | go1.24.9 | Quadratic complexity when checking name constraints |
| GO-2025-3750 | os / syscall | go1.24.4 | Inconsistent handling of O_CREATE and O_EXCL on Unix and Windows |
| GO-2025-3749 | crypto/x509 | go1.24.4 | ExtKeyUsageAny disables policy validation |
| GO-2025-3563 | net/http/internal | go1.24.2 | Request smuggling due to acceptance of invalid chunked data |
| GO-2025-3503 | net/http | go1.24.1 | HTTP proxy bypass using IPv6 Zone IDs |

---

## Appendice C — Scenari di riproduzione

L'harness riproduce la pipeline di `cmd/main.go` (`LoggingMiddleware` → `DynamicProxyHandler`) davanti a un backend `httptest`. La config viene caricata con `config.LoadConfiguration`, quindi con i default reali (per esempio il limite di 100 MB sulle risposte).

| ID | Finding | Configurazione | Backend | Richiesta | Atteso | Osservato |
|---|---|---|---|---|---|---|
| R-01 | F-01 | `^/big$` → `BACKEND/big`, `replace_path: true` | 204.800 B con `Content-Length` | `GET /big` | 200, 204.800 B | 200, `Content-Length: 3975`, 3.975 B |
| R-02 | F-02, F-06 | `^/echo$` → `BACKEND/echo` | restituisce la lunghezza del body | `POST` form `a=1&b=2` | il backend riceve 7 B | 502 con `upstream_error`: `ContentLength=7 with Body length 0` |
| R-03 | F-02 | `^/q$` → `BACKEND/q` | 200 | `GET /q?a=%zz` | inoltrata al backend | 400 `Invalid request format` |
| R-04 | F-03 | `^/up$` → `BACKEND/up` | conta i byte ricevuti | `POST` chunked da 11 MiB | 413 | 200; il backend riceve 11.534.336 B |
| R-05 | F-04 | `^/api` → `BACKEND/base`, `replace_path: false` | restituisce il path | `GET /api/users` | `/base/users` | `/base/api/users` |
| R-06 | F-05 | `^/echo$` → `BACKEND/echo` | restituisce gli header | `GET` con `Host: public.example.com` | XFF `127.0.0.1`; XFH `public.example.com` | XFF `127.0.0.1, 127.0.0.1:50476`; XFH = host del backend |
| R-07 | F-13 | `^/ws$` → `ws://BACKEND/ws`, `enable_websocket: true`, `middlewares: [auth]` | echo WebSocket | GET normale; upgrade WebSocket | 401; 401 | 401; connesso, echo `ping` |
| R-08 | F-28, F-29 | `^/echo$`, metriche attive | — | 1 GET e 1 POST | contatore 1 per ciascuno | contatore 2 per ciascuno, label `status_code="OK"` |
| R-09 | F-23 | come R-06 | — | 4 client in loop e 20 reload, con `-race` | nessuna race | 34 `DATA RACE` |
| R-10 | F-24 | file di config con `port` | — | modifiche 1111 → 2222 → 1111 | `onChange` per 2222 e per 1111 | solo per 2222 |
| R-11 | F-32 | `logging.enabled: false` | — | 1 richiesta | nessun log | log compatto presente |
| R-12 | F-32 | `enabled: true`, `verbose: true`, livello info | — | 1 richiesta | un log della richiesta | nessun log |
| R-13 | F-35 | binario reale, config senza `plugins` | — | avvio | Dito parte | exit 1 (`failed to read public key`) |
| R-14 | F-01 | binario reale, `^/big$` → `python3 -m http.server`, file da 200 KB | — | `curl /big` | 204.800 B | status 200, 16.332 B |

<details>
<summary>Harness usato (condensato)</summary>

```go
// package handlers_test: stessa pipeline di cmd/main.go.
func newFront(t *testing.T, backend http.Handler, locations string, plugins ...plugin.Plugin) (string, *app.Dito, string) {
	t.Helper()
	be := httptest.NewServer(backend)
	t.Cleanup(be.Close)
	yaml := "port: \"0\"\n" +
		"logging: {enabled: true, level: \"error\"}\n" +
		"metrics: {enabled: true, path: \"/metrics\"}\n" +
		"transport: {http: {dial_timeout: 2s, response_header_timeout: 10s}}\n" +
		"locations:\n" + strings.ReplaceAll(locations, "BACKEND", be.URL)
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfiguration(cfgFile) // applica i default reali
	if err != nil {
		t.Fatal(err)
	}
	config.UpdateConfig(cfg)
	dito := app.NewDito(&cfg.Transport.HTTP, logging.InitializeLogger("error"))
	front := httptest.NewServer(cmid.LoggingMiddleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { handlers.DynamicProxyHandler(dito, w, r, plugins) }), dito))
	t.Cleanup(front.Close)
	return front.URL, dito, cfgFile
}

// R-07: plugin finto che nega tutto.
type denyPlugin struct{}

func (denyPlugin) Name() string                                                  { return "auth" }
func (denyPlugin) Init(context.Context, map[string]any, plugin.AppAccessor) error { return nil }
func (denyPlugin) MiddlewareFunc() func(http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "denied", http.StatusUnauthorized)
		})
	}
}

// R-09: 4 goroutine fanno GET in loop mentre la config viene ricaricata 20 volte (con -race).
for range 20 {
	newCfg, _ := config.LoadConfiguration(cfgFile)
	dito.UpdateComponents(newCfg)
	dito.UpdateConfig(newCfg)
	time.Sleep(5 * time.Millisecond)
}

// R-10: come cmd/main.go, onChange aggiorna solo l'istanza e mai la config globale.
go config.WatchConfig(file, func(c *config.ProxyConfig) { applied = append(applied, c.Port) }, logger)
// Scrivere 1111 → 2222 → 1111 aggiornando l'mtime (os.Chtimes) e attendere ~4 s tra una modifica e l'altra.

// R-11/R-12 (package middlewares_test): Dito con un logger slog che scrive su un buffer,
// una richiesta attraverso LoggingMiddleware, attesa dei worker asincroni, verifica del contenuto.
```

</details>

<details>
<summary>Procedura E2E (R-13, R-14, A-10, A-12)</summary>

```sh
export GOTOOLCHAIN=go1.27.1
go build -o $E/dito ./cmd
(cd cmd/plugin-signer && go build -o $E/plugin-signer .)
(cd plugins/hello-plugin && go build -buildmode=plugin -o $E/plugins/hello-plugin/hello-plugin.so .)
cp plugins/hello-plugin/config.yaml $E/plugins/hello-plugin/
cd $E && ./plugin-signer generate-keys && ./plugin-signer sign plugins/hello-plugin/hello-plugin.so
# config.yaml: plugins.public_key_hash = sha256 di ed25519_public.key;
#              location ^/small$ con middleware hello-plugin, ^/big$ verso un file da 200 KB
python3 -m http.server 18090 --bind 127.0.0.1 --directory www &
./dito -f config.yaml &
curl -s -D - http://127.0.0.1:18081/small                               # 200 + X-Hello-Plugin
curl -s -o /dev/null -w '%{size_download}\n' http://127.0.0.1:18081/big # 16332 invece di 204800

# Immagine
podman build --platform linux/amd64 -t dito:go1.27-test .
# go version sui binari estratti → go1.27.1; nel container: generate-keys, sign, avvio,
# wget di una location con hello-plugin → 200 + X-Hello-Plugin
```

</details>

---

## Appendice D — Analisi statica

Comando:

```sh
golangci-lint run --enable gosec,bodyclose,errorlint,noctx,modernize,unparam,contextcheck,nilerr,copyloopvar \
  --max-issues-per-linter 0 --max-same-issues 0 ./...
```

| Linter | Issue | Esempi rilevanti |
|---|---|---|
| errcheck | 38 | `Write`, `Close` e `io.Copy` senza controllo dell'errore |
| staticcheck | 26 | SA1019 `ReverseProxy.Director` deprecato; SA4023 confronti sempre veri in `websocket.go`; SA9003 rami vuoti in `middlewares/logging.go`; QF1012 in `logging.go` |
| gosec | 16 | G112 Slowloris (`cmd/main.go:126`); G108 pprof; G304 path da variabile (plugin, config, signer); G306 permessi 0644 nel signer |
| errorlint | 9 | `%v` invece di `%w` in `config.go` e `transport.go` |
| unparam | 7 | parametri inutilizzati (es. `dito` in `createResponseWriterWithLimits`) |
| noctx | 4 | richieste senza context |
| bodyclose | 2 | `handlers.go:218`, `websocket.go:45` |
| unused | 2 | `printTransportDetails`, `createResponseWriter` in `middlewares` |
| contextcheck | 1 | `createResponseModifier` |

Codice morto emerso dall'analisi manuale:

- `LogResponse`, `LogResponseMetrics` e `GetBufferedBody` non sono mai chiamati in produzione;
- `InvalidateTransport` e `genericTransport` non sono usati;
- le costanti `maxHeaderSize`, `headerXRealIP` e `headerXRateLimit*` sono inutilizzate;
- 11 funzioni di registrazione delle metriche non sono mai chiamate (F-31).

---

## Appendice E — Sospetti smentiti e limiti dell'analisi

**Sospetti smentiti**

- **X-01 — Early Hints**: con un backend che risponde `103` e poi `404`, il client riceve correttamente `404`. Non è stato verificato se il `103` intermedio arrivi al client.

**Limiti dell'analisi**

- I manifest Kubernetes e OpenShift non sono stati eseguiti su un cluster: le conclusioni di F-17 derivano dalla lettura dei manifest.
- F-14 non è stato sfruttato attivamente (CSWSH, messaggi enormi): l'evidenza è la lettura del codice.
- Nessun benchmark: gli aspetti di performance (F-09, F-10, F-25, F-30) sono stimati dal codice, non misurati.
- L'harness di riproduzione era temporaneo e non fa parte del repository: i test di regressione vanno scritti nelle rispettive spec, a partire dagli scenari R-xx.
