---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T20:14:15Z
last_modified: 2026-09-30T20:14:15Z
approved_fingerprint: sha256:da14101a393ee58ac70c16bc4893e7be9efc4899d07803cc5c5e7112a702dd9f
source_design_approved_at: 2026-09-30T20:03:50Z
source_design_fingerprint: sha256:b332b26e2edca7b60eb0e5a377a66aeb48c43a3ec64364b2a5a428592ea964bb
---

# Implementation Plan

Note comuni a tutti i task:

- **Toolchain**: le proof eseguono `make` oppure `env GOTOOLCHAIN=auto go ...`, e funzionano anche con il Go locale 1.24.
- **`scripts/ci/expect-output.sh`** (task 1.2): esegue una volta il comando dopo `--` e stampa `expect-output: ok` solo se il comando esce con 0, se l'output contiene ogni testo `--contains` e se non contiene nessun testo `--absent`. Altrimenti stampa l'output e i testi mancanti o presenti ed esce con 1.
- **Proof di un'area**: `make e2e SCENARIO=^Test<Area>_` e poi tre verifiche:
  - `=== RUN` per ogni scenario del catalogo;
  - uscita 0, cioè nessun fallimento inatteso;
  - solo i fatti già verificati dalle sonde P1-P12 e dal PRD: i `KNOWN BUG F-xx` riprodotti e i `PASS` osservati.

  Le previsioni del design non ancora verificate non entrano nelle proof.
- **Scenario diverso dalla previsione**:
  - se è un bug dell'harness, si corregge;
  - se è un finding nuovo, riceve un ID nuovo (`F-57` e seguenti), va registrato nel PRD locale e lo scenario viene marcato;
  - se passa dove era previsto un bug noto, non si marca.

  Se una proof che afferma un fatto verificato fallisce, ci si ferma e si ripianifica.
- **Ordine di lavoro**: prima i test dell'harness, che devono fallire, poi il codice. Gli scenari del catalogo documentano il comportamento: si scrivono, si eseguono, e si marcano secondo l'esito osservato.
- **Sola lettura**: binari, plugin, chiavi e config temporanee stanno in directory temporanee fuori dal repository.
- **Lint**: il codice nuovo passa `make lint`. Le soppressioni sono puntuali, nella forma `//nolint:<linter> // <motivo>`.

- [ ] 1. Harness
  - [ ] 1.1 Catena condivisa `handlers.NewHandler`
    - `NewHandler(dito, plugins) http.Handler` restituisce il `mux` con `"/"` → `LoggingMiddleware(DynamicProxyHandler)`. `cmd/main.go` la usa al posto del `mux` costruito sul posto; il comportamento non cambia.
    - Prima `TestNewHandler`: una richiesta attraverso `NewHandler` arriva a un backend `httptest` e il client riceve il suo body.
    - Requirements: `C2`
    - Design: Architecture
    - Verification:
      - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-v", "-run", "^TestNewHandler$", "./handlers/"]
        expect_output: "--- PASS: TestNewHandler"
      - command: ["make", "build-check"]
        expect_output: "build-check: ok"
  - [ ] 1.2 `Run`, bug noti, riepilogo e `make e2e`
    - `scripts/ci/expect-output.sh`, target `make e2e` (`go test -count=1 -race -shuffle=on -v ./e2e/`, con `SCENARIO` passato a `-run`), package `e2e`:
      - `Run`, `S`, `KnownBug`;
      - registro dei risultati e riepilogo in `TestMain`;
      - interfaccia `TB` minima e `TB` finto;
      - diagnostica dei fallimenti.
    - Prima i test dell'harness con il `TB` finto:
      - uno scenario marcato che fallisce risulta skip con `KNOWN BUG`;
      - uno scenario marcato che passa fallisce;
      - un errore dell'harness in uno scenario marcato fallisce;
      - il riepilogo raggruppa per finding;
      - un fallimento provocato mostra richieste, risposte e log.
    - Requirements: `R4.AC1`, `R4.AC2`, `R4.AC3`, `R4.AC4`, `R2.AC8`, `NFR4`
    - Design: Architecture (Bug noti, Esecuzione), Verification Plan
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "hello", "--", "echo", "hello"]
        expect_output: "expect-output: ok"
      - command: ["scripts/ci/expect-output.sh", "--contains", "absent-text", "--", "echo", "hello"]
        expect_exit: 1
        expect_output: "missing: absent-text"
      - command: ["scripts/ci/expect-output.sh", "--contains", "--- PASS: TestHarness_KnownBugReported", "--contains", "--- PASS: TestHarness_KnownBugFixedFails", "--contains", "--- PASS: TestHarness_KnownBugInfraErrorFails", "--contains", "--- PASS: TestHarness_SummaryByFinding", "--contains", "--- PASS: TestHarness_FailureDiagnostics", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestHarness_(KnownBug|Summary|Failure)"]
        expect_output: "expect-output: ok"
        covers: ["R4.AC1", "R4.AC2", "R4.AC3", "R4.AC4", "R2.AC8"]
        timeout: 20m
  - [ ] 1.3 Backend, client e osservazioni
    - Backend `httptest` che registrano le richieste e servono body deterministici. Client con framing, `1xx`, hash in streaming, tempi di arrivo e interruzioni. Client WebSocket.
    - Prima i test dell'harness, eseguiti direttamente contro i backend:
      - un body da 64 MiB confrontato per hash;
      - `Content-Length` e chunked;
      - un `103` prima della risposta finale;
      - il primo evento SSE che arriva mentre il backend attende;
      - un'interruzione a metà body;
      - le richieste registrate dal backend;
      - l'echo WebSocket.
    - Requirements: `R2.AC1`, `R2.AC2`, `R2.AC3`, `R2.AC4`, `R2.AC5`, `R2.AC6`
    - Design: Architecture (Osservazioni)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "--- PASS: TestHarness_Observe64MiBByHash", "--contains", "--- PASS: TestHarness_ObserveFraming", "--contains", "--- PASS: TestHarness_ObserveEarlyHints", "--contains", "--- PASS: TestHarness_ObserveStreamingTiming", "--contains", "--- PASS: TestHarness_ObserveAbort", "--contains", "--- PASS: TestHarness_BackendRecordsRequests", "--contains", "--- PASS: TestHarness_WebSocketEcho", "--", "make", "e2e", "SCENARIO=^TestHarness_(Observe|Backend|WebSocket)"]
        expect_output: "expect-output: ok"
        covers: ["R2.AC1", "R2.AC2", "R2.AC3", "R2.AC4", "R2.AC5", "R2.AC6"]
        timeout: 20m
  - [ ] 1.4 Proxy in-process
    - Template della config (`{{backend "nome"}}`, `{{ws "nome"}}`, `{{ca "nome"}}`, `{{clientCert}}`, `{{clientKey}}`), caricamento con `config.LoadConfiguration`, logger catturato, plugin finti, catena di `handlers.NewHandler` servita da `httptest`.
    - Prima i test dell'harness:
      - una location inoltra al backend dello scenario;
      - la config renderizzata non contiene porte fisse e usa gli indirizzi dei backend avviati;
      - un log di accesso del proxy è leggibile dallo scenario.
    - Requirements: `R1.AC2`, `R2.AC7`, `C3`
    - Design: Architecture (Proxy in-process)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "--- PASS: TestHarness_InProcessProxy", "--contains", "--- PASS: TestHarness_ConfigTemplateUsesBackends", "--contains", "--- PASS: TestHarness_ProxyLogsCaptured", "--", "make", "e2e", "SCENARIO=^TestHarness_(InProcess|ConfigTemplate|ProxyLogs)"]
        expect_output: "expect-output: ok"
        covers: ["R1.AC2", "R2.AC7"]
        timeout: 20m
  - [ ] 1.5 Binario
    - Build unica per processo di test (proxy normale e `-race`, `plugin-signer`, plugin) con `GOTOOLCHAIN=go<versione di go.mod>`. Porta libera con al più tre tentativi, attesa della prontezza, log catturati, arresto con `SIGTERM` e poi `SIGKILL`. `TestMain` fallisce se resta un processo figlio.
    - Prima i test dell'harness:
      - lo stesso scenario dà lo stesso esito in-process e con il binario;
      - il binario viene compilato nella stessa esecuzione;
      - un proxy che non parte fa fallire lo scenario entro il tempo massimo, con i log.

      La seconda proof ripete la suite dell'harness nello stesso processo (`-count=2`), senza processi residui né porte occupate.
    - Requirements: `R1.AC1`, `R1.AC3`, `R1.AC4`, `R1.AC5`, `C2`, `C4`
    - Design: Architecture (Binario), Failure Modes And Tradeoffs
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "--- PASS: TestHarness_SameOutcomeInProcessAndBinary", "--contains", "--- PASS: TestHarness_BinaryBuiltInRun", "--contains", "--- PASS: TestHarness_ProxyNotReadyReportsLogs", "--", "make", "e2e", "SCENARIO=^TestHarness_(SameOutcome|BinaryBuilt|ProxyNotReady)"]
        expect_output: "expect-output: ok"
        covers: ["R1.AC1", "R1.AC3", "R1.AC4"]
        timeout: 20m
      - command: ["scripts/ci/expect-output.sh", "--contains", "e2e summary:", "--absent", "--- FAIL", "--", "env", "GOTOOLCHAIN=auto", "go", "test", "-count=2", "-v", "-run", "^TestHarness_", "./e2e/"]
        expect_output: "expect-output: ok"
        covers: ["R1.AC5"]
        timeout: 30m

- [ ] 2. Catalogo degli scenari
  - [ ] 2.1 Routing
    - Scenari: `TestRouting_RegexMatch`, `TestRouting_NoLocation404`, `TestRouting_FirstLocationWins`, `TestRouting_ReplacePathTrue`, `TestRouting_R05_ReplacePathFalse`, `TestRouting_QueryPreserved`. Previsto: F-04 per `R-05` (verificato).
    - Requirements: `R3.AC1`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestRouting_RegexMatch", "--contains", "=== RUN   TestRouting_NoLocation404", "--contains", "=== RUN   TestRouting_FirstLocationWins", "--contains", "=== RUN   TestRouting_ReplacePathTrue", "--contains", "=== RUN   TestRouting_R05_ReplacePathFalse", "--contains", "=== RUN   TestRouting_QueryPreserved", "--contains", "KNOWN BUG F-04", "--contains", "--- PASS: TestRouting_ReplacePathTrue", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestRouting_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC1"]
        timeout: 20m
  - [ ] 2.2 Header
    - Scenari: `TestHeaders_R06_XForwarded`, `TestHeaders_HostRewritten`, `TestHeaders_RequestIDGenerated`, `TestHeaders_RequestIDPropagated`, `TestHeaders_HopByHopRemoved`, `TestHeaders_AdditionalHeaders`, `TestHeaders_ExcludedHeaders`, `TestHeaders_ExcludedHeadersCaseInsensitive`, `TestHeaders_SecurityHeaders`. Previsti: F-05 per `R-06` (verificato), F-12 per `excluded_headers` senza distinzione di maiuscole (da verificare).
    - Requirements: `R3.AC2`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestHeaders_R06_XForwarded", "--contains", "=== RUN   TestHeaders_HostRewritten", "--contains", "=== RUN   TestHeaders_RequestIDGenerated", "--contains", "=== RUN   TestHeaders_RequestIDPropagated", "--contains", "=== RUN   TestHeaders_HopByHopRemoved", "--contains", "=== RUN   TestHeaders_AdditionalHeaders", "--contains", "=== RUN   TestHeaders_ExcludedHeaders", "--contains", "=== RUN   TestHeaders_ExcludedHeadersCaseInsensitive", "--contains", "=== RUN   TestHeaders_SecurityHeaders", "--contains", "KNOWN BUG F-05", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestHeaders_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC2"]
        timeout: 20m
  - [ ] 2.3 Body della richiesta
    - Scenari: `TestRequestBody_JSONIntact`, `TestRequestBody_R02_FormPost`, `TestRequestBody_R03_MalformedQuery`, `TestRequestBody_LimitWithContentLength`, `TestRequestBody_R04_LimitChunked`, `TestRequestBody_LargeUnderLimit`. Previsti e verificati: F-02 per `R-02` e `R-03`, F-03 per `R-04`.
    - Requirements: `R3.AC3`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestRequestBody_JSONIntact", "--contains", "=== RUN   TestRequestBody_R02_FormPost", "--contains", "=== RUN   TestRequestBody_R03_MalformedQuery", "--contains", "=== RUN   TestRequestBody_LimitWithContentLength", "--contains", "=== RUN   TestRequestBody_R04_LimitChunked", "--contains", "=== RUN   TestRequestBody_LargeUnderLimit", "--contains", "KNOWN BUG F-02", "--contains", "KNOWN BUG F-03", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestRequestBody_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC3"]
        timeout: 20m
  - [ ] 2.4 Body della risposta
    - Scenari: `TestResponseBody_Empty`, `TestResponseBody_OneByte`, `TestResponseBody_StatusWithEmptyBody`, `TestResponseBody_R01_P01_LargeWithContentLength`, `TestResponseBody_P02_LargeChunked`, `TestResponseBody_P05_HeadOverLimit`, `TestResponseBody_NotModifiedOverLimit`, `TestResponseBody_P06_EarlyHintsForwarded`, `TestResponseBody_P06_FinalStatusAfterEarlyHints`, `TestResponseBody_P07_ServerSentEvents`, `TestResponseBody_P10_MemoryBounded`, `TestResponseBody_P11_BackendAbortsBody`.
    - Verificati: F-01 (`R-01`, P2, P7, P10), F-55 (P5), F-56 (P6: `103`), status finale corretto dopo il `103`, interruzione visibile al client (P11).
    - Da verificare: `304` oltre il limite (previsto F-55) e status con body vuoto.
    - Requirements: `R3.AC4`
    - Design: Architecture (Catalogo iniziale, Finding nuovi)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestResponseBody_Empty", "--contains", "=== RUN   TestResponseBody_OneByte", "--contains", "=== RUN   TestResponseBody_StatusWithEmptyBody", "--contains", "=== RUN   TestResponseBody_R01_P01_LargeWithContentLength", "--contains", "=== RUN   TestResponseBody_P02_LargeChunked", "--contains", "=== RUN   TestResponseBody_P05_HeadOverLimit", "--contains", "=== RUN   TestResponseBody_NotModifiedOverLimit", "--contains", "=== RUN   TestResponseBody_P06_EarlyHintsForwarded", "--contains", "=== RUN   TestResponseBody_P06_FinalStatusAfterEarlyHints", "--contains", "=== RUN   TestResponseBody_P07_ServerSentEvents", "--contains", "=== RUN   TestResponseBody_P10_MemoryBounded", "--contains", "=== RUN   TestResponseBody_P11_BackendAbortsBody", "--contains", "KNOWN BUG F-01", "--contains", "KNOWN BUG F-55", "--contains", "KNOWN BUG F-56", "--contains", "--- PASS: TestResponseBody_P06_FinalStatusAfterEarlyHints", "--contains", "--- PASS: TestResponseBody_P11_BackendAbortsBody", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestResponseBody_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC4"]
        timeout: 20m
  - [ ] 2.5 Limiti di risposta
    - Scenari: `TestLimits_P03_DeclaredOverLimit`, `TestLimits_P04_OverLimitAfterStart`, `TestLimits_P09_ErrorJSONForAnyPath`, `TestLimits_P12_FirstChunkOverLimit`, `TestLimits_LocationLimitOverridesGlobal`, `TestLimits_GlobalLimitWithoutLocationLimit`, `TestLimits_WarningLogged`. Verificati: F-54 (P3, P12), F-01 (P4: risposta troncata che sembra completa). Previsti: F-54 per limiti di location e globale, F-54 e F-07 per P9.
    - Requirements: `R3.AC5`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestLimits_P03_DeclaredOverLimit", "--contains", "=== RUN   TestLimits_P04_OverLimitAfterStart", "--contains", "=== RUN   TestLimits_P09_ErrorJSONForAnyPath", "--contains", "=== RUN   TestLimits_P12_FirstChunkOverLimit", "--contains", "=== RUN   TestLimits_LocationLimitOverridesGlobal", "--contains", "=== RUN   TestLimits_GlobalLimitWithoutLocationLimit", "--contains", "=== RUN   TestLimits_WarningLogged", "--contains", "KNOWN BUG F-54", "--contains", "KNOWN BUG F-01", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestLimits_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC5"]
        timeout: 20m
  - [ ] 2.6 Errori verso il backend
    - Scenari: `TestErrors_BackendUnreachable`, `TestErrors_BackendTimeout`, `TestErrors_InvalidTarget`, `TestErrors_ClientCancels`. Esiti da verificare.
    - Requirements: `R3.AC6`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestErrors_BackendUnreachable", "--contains", "=== RUN   TestErrors_BackendTimeout", "--contains", "=== RUN   TestErrors_InvalidTarget", "--contains", "=== RUN   TestErrors_ClientCancels", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestErrors_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC6"]
        timeout: 20m
  - [ ] 2.7 Plugin e middleware
    - Scenari: `TestPlugins_MiddlewareOrder`, `TestPlugins_CriticalMiddlewareMissing`, `TestPlugins_SignedPluginLoaded`, `TestPlugins_TamperedPluginRejected`. Verificati dallo smoke e da `make ci-selftest` di S-02: plugin firmato caricato, plugin manomesso rifiutato.
    - Requirements: `R3.AC7`, `C4`
    - Design: Architecture (Catalogo iniziale, Binario)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestPlugins_MiddlewareOrder", "--contains", "=== RUN   TestPlugins_CriticalMiddlewareMissing", "--contains", "=== RUN   TestPlugins_SignedPluginLoaded", "--contains", "=== RUN   TestPlugins_TamperedPluginRejected", "--contains", "--- PASS: TestPlugins_SignedPluginLoaded", "--contains", "--- PASS: TestPlugins_TamperedPluginRejected", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestPlugins_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC7"]
        timeout: 20m
  - [ ] 2.8 WebSocket
    - Scenario `TestWebSocket_Echo` verso un backend `ws` (verificato: l'echo passa, R-07). Gli scenari di sicurezza restano fuori (`C6`).
    - Requirements: `R3.AC8`, `C6`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestWebSocket_Echo", "--contains", "--- PASS: TestWebSocket_Echo", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestWebSocket_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC8"]
        timeout: 20m
  - [ ] 2.9 Transport e TLS verso i backend
    - Scenari: `TestTransport_HTTPSBackendCustomCA`, `TestTransport_MutualTLS`, `TestTransport_LocationOverridesGlobal`, con certificati generati durante il test (`certs_test.go`). Esiti da verificare.
    - Requirements: `R3.AC9`
    - Design: Architecture (Catalogo iniziale)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestTransport_HTTPSBackendCustomCA", "--contains", "=== RUN   TestTransport_MutualTLS", "--contains", "=== RUN   TestTransport_LocationOverridesGlobal", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestTransport_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC9"]
        timeout: 20m
  - [ ] 2.10 Hot reload (binario)
    - Scenari: `TestReload_NewLocationServed`, `TestReload_R10_RevertDetected`, `TestReload_R09_ConcurrentRequestsRace`. `R-10` cambia il target di una location da A a B e poi di nuovo ad A. `R-09` usa il binario `-race` con client concorrenti e al più tre reload.
    - Previsti: F-24 e F-23, verificati in-process durante l'assessment. Col binario vanno verificati, quindi la proof non li afferma.
    - Requirements: `R3.AC10`
    - Design: Architecture (Binario), Failure Modes And Tradeoffs
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestReload_NewLocationServed", "--contains", "=== RUN   TestReload_R10_RevertDetected", "--contains", "=== RUN   TestReload_R09_ConcurrentRequestsRace", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestReload_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC10"]
        timeout: 20m
  - [ ] 2.11 Metriche
    - Scenari: `TestMetrics_EndpointExposed`, `TestMetrics_R08_CountersPerRequest`, con i contatori confrontati per differenza. Verificati: endpoint esposto (smoke di S-02), F-28 per `R-08`.
    - Requirements: `R3.AC11`
    - Design: Architecture (Osservazioni)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestMetrics_EndpointExposed", "--contains", "=== RUN   TestMetrics_R08_CountersPerRequest", "--contains", "KNOWN BUG F-28", "--contains", "--- PASS: TestMetrics_EndpointExposed", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestMetrics_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC11"]
        timeout: 20m
  - [ ] 2.12 Logging
    - Scenari: `TestLogging_CompactAccessLog`, `TestLogging_R11_Disabled`, `TestLogging_R12_Verbose`. Verificato: F-32 per `R-11` e `R-12`.
    - Requirements: `R3.AC12`
    - Design: Architecture (Osservazioni)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestLogging_CompactAccessLog", "--contains", "=== RUN   TestLogging_R11_Disabled", "--contains", "=== RUN   TestLogging_R12_Verbose", "--contains", "KNOWN BUG F-32", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestLogging_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC12"]
        timeout: 20m
  - [ ] 2.13 Ciclo di vita (binario)
    - Scenari: `TestLifecycle_R13_StartWithoutPlugins`, `TestLifecycle_GracefulShutdown`, `TestLifecycle_R14_LargeResponseThroughBinary`. Verificati: F-35 per `R-13`, F-01 per `R-14`.
    - Requirements: `R3.AC13`
    - Design: Architecture (Binario)
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestLifecycle_R13_StartWithoutPlugins", "--contains", "=== RUN   TestLifecycle_GracefulShutdown", "--contains", "=== RUN   TestLifecycle_R14_LargeResponseThroughBinary", "--contains", "KNOWN BUG F-35", "--contains", "KNOWN BUG F-01", "--contains", "e2e summary:", "--", "make", "e2e", "SCENARIO=^TestLifecycle_"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC13"]
        timeout: 20m
  - [ ] 2.14 Tracciabilità del catalogo
    - `go test -list` contiene gli ID `R-01`…`R-14` tranne `R-07` (`C6`) e le sonde citate in `R3.AC4` e `R3.AC5`.
    - Requirements: `R3.AC14`, `C6`
    - Design: Architecture, Verification Plan
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "_R01_", "--contains", "_R02_", "--contains", "_R03_", "--contains", "_R04_", "--contains", "_R05_", "--contains", "_R06_", "--contains", "_R08_", "--contains", "_R09_", "--contains", "_R10_", "--contains", "_R11_", "--contains", "_R12_", "--contains", "_R13_", "--contains", "_R14_", "--contains", "_P01_", "--contains", "_P02_", "--contains", "_P03_", "--contains", "_P04_", "--contains", "_P05_", "--contains", "_P06_", "--contains", "_P07_", "--contains", "_P09_", "--contains", "_P10_", "--contains", "_P11_", "--contains", "_P12_", "--absent", "_R07_", "--", "env", "GOTOOLCHAIN=auto", "go", "test", "-list", ".*", "./e2e/"]
        expect_output: "expect-output: ok"
        covers: ["R3.AC14"]

- [ ] 3. Integrazione
  - [ ] 3.1 Suite nei controlli esistenti
    - `test-race`, `test-hermetic` e `coverage` eseguono i package unitari come oggi e poi `./e2e/` con `-v`, in un'invocazione separata. `test-hermetic` scarica prima, fuori dall'isolamento, anche le dipendenze del modulo del plugin.
    - Requirements: `R5.AC3`, `R5.AC4`, `NFR1`
    - Design: Architecture (Esecuzione), Failure Modes And Tradeoffs
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "e2e summary:", "--contains", "test-race: ok", "--", "make", "test-race"]
        expect_output: "expect-output: ok"
        covers: ["R5.AC4"]
        timeout: 30m
      - command: ["scripts/ci/expect-output.sh", "--contains", "e2e summary:", "--contains", "test-hermetic: ok", "--", "make", "test-hermetic"]
        expect_output: "expect-output: ok"
        covers: ["R5.AC4"]
        timeout: 30m
      - command: ["scripts/ci/expect-output.sh", "--contains", "e2e summary:", "--contains", "coverage: ok", "--", "make", "coverage"]
        expect_output: "expect-output: ok"
        timeout: 30m
  - [ ] 3.2 `make e2e`: filtro e durata
    - Il filtro `SCENARIO` esegue solo l'area richiesta. L'intera suite termina in al più 120 s con la cache calda.
    - Requirements: `R5.AC1`, `R5.AC2`, `NFR2`
    - Design: Architecture (Esecuzione), Verification Plan
    - Verification:
      - command: ["scripts/ci/expect-output.sh", "--contains", "=== RUN   TestLimits_", "--contains", "e2e summary:", "--absent", "=== RUN   TestRouting_", "--", "make", "e2e", "SCENARIO=^TestLimits_"]
        expect_output: "expect-output: ok"
        covers: ["R5.AC2"]
        timeout: 20m
      - command: ["sh", "-c", "make e2e >/dev/null 2>&1 || exit 1; start=$(date +%s); out=$(make e2e 2>&1) || { printf '%s\n' \"$out\"; exit 1; }; d=$(( $(date +%s) - start )); case \"$out\" in *'e2e summary:'*) ;; *) exit 1 ;; esac; echo \"e2e duration ${d}s\"; [ \"$d\" -le 120 ]"]
        expect_output: "e2e duration"
        covers: ["R5.AC1"]
        timeout: 20m
  - [ ] 3.3 Casi di `make ci-selftest`
    - Due casi nuovi in `scripts/ci/selftest.sh`:
      - `e2e-regression`: toglie `req.Host = targetURL.Host` da `createDirector` e verifica che `make test-race` fallisca citando `TestHeaders_HostRewritten`;
      - `e2e-new-scenario`: aggiunge soltanto `e2e/zz_selftest_scenario_test.go`, con uno scenario che fallisce, e verifica che `make e2e` fallisca citandolo.
    - Requirements: `R5.AC5`, `R6.AC1`
    - Design: Architecture (Esecuzione)
    - Verification:
      - command: ["make", "ci-selftest", "SELFTEST_CASES=e2e-regression e2e-new-scenario"]
        expect_output: "selftest summary: 2 ok, 0 failed"
        covers: ["R5.AC5", "R6.AC1"]
        timeout: 40m
  - [ ] 3.4 Modo `e2e` di `observe-github.sh`
    - `observe-github.sh e2e` legge i log dell'ultima esecuzione di `ci.yml` per una PR e verifica che i job `test`, `hermetic` e `coverage` contengano il riepilogo della suite. Confronta il log catturato, senza `grep -q` in pipeline (lezione registrata). La verifica reale avviene dopo il push (4.2).
    - Requirements: `R5.AC3`
    - Design: Verification Plan
    - Verification:
      - command: ["scripts/ci/observe-github.sh", "bogus"]
        expect_exit: 2
        expect_output: "runs|schedule|renovate|e2e"
  - [ ] 3.5 README e PRD locale
    - Sezione del README sugli scenari end-to-end: come scriverne uno, eseguirlo (`make e2e`, `SCENARIO`) e marcare o togliere un bug noto, con un esempio completo.
    - Registrazione di F-54, F-55, F-56 e degli eventuali finding nuovi nel PRD locale. Il PRD non è versionato, quindi nessuna proof lo verifica.
    - Requirements: `R6.AC2`
    - Design: Architecture, Verification Plan
    - Verification:
      - command: ["grep", "-q", "make e2e SCENARIO=", "README.md"]
      - command: ["grep", "-q", "KnownBug(", "README.md"]
        covers: ["R6.AC2"]

- [ ] 4. Chiusura
  - [ ] 4.1 Checkpoint locale completo
    - `make ci`, l'harness di mutazione completo (24 casi di S-02 più i 2 nuovi) e 10 esecuzioni consecutive della suite in ordine casuale.
    - Requirements: `R5.AC4`, `NFR2`, `NFR3`
    - Design: Verification Plan
    - Verification:
      - command: ["make", "ci"]
        expect_output: "ci: ok"
        timeout: 60m
      - command: ["make", "ci-selftest"]
        expect_output: "selftest summary: 26 ok, 0 failed"
        timeout: 90m
      - command: ["scripts/ci/expect-output.sh", "--contains", "e2e summary:", "--absent", "--- FAIL", "--", "env", "GOTOOLCHAIN=auto", "go", "test", "-count=10", "-shuffle=on", "-v", "./e2e/"]
        expect_output: "expect-output: ok"
        covers: ["R5.AC4"]
        timeout: 90m
  - [ ] 4.2 Osservazione su GitHub
    - Da completare dopo il push e la PR verso `main`: `observe-github.sh e2e` sull'esecuzione della PR.
    - Requirements: `R5.AC3`
    - Design: Verification Plan
    - Verification:
      - command: ["scripts/ci/observe-github.sh", "e2e"]
        expect_output: "observe e2e: ok"
        covers: ["R5.AC3"]
