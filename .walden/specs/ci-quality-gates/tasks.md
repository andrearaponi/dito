---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T18:07:22Z
last_modified: 2026-09-30T18:07:22Z
approved_fingerprint: sha256:8f3d9187cf7e574ad2d4d4fb5df7074c7694a160eae1e75e4b40247ef6c543b9
source_design_approved_at: 2026-09-30T07:00:40Z
source_design_fingerprint: sha256:3d9aa995ab487513125606401422d095b83b782bd3c3abd58399dbba5acc78aa
---

# Implementation Plan

Note comuni a tutti i task:

- **Toolchain**: le proof eseguono i target `make`, che esportano `GOTOOLCHAIN=auto` e quindi funzionano anche con il Go locale 1.24.
- **Marker**: ogni target stampa `<target>: ok` solo se tutti i suoi controlli sono passati.
- **Harness `scripts/ci/selftest.sh`**: esegue ogni caso in un repository temporaneo. Un caso è ok solo se, a seconda di quanto dichiara:
  - il target passa sull'istantanea non modificata;
  - dopo la mutazione il target fallisce, oppure passa per i casi limite;
  - l'output contiene o non contiene i testi indicati.

  In questo modo un target mancante o rotto non può far passare un caso.
- **TDD**: prima il caso di mutazione o il test (rosso), poi l'implementazione.
- **Sola lettura**: nessuna proof modifica il working tree. Output, profili e repository temporanei stanno fuori dal repository.

- [x] 1. Fondamenta
  - [x] 1.1 `tools.mk` e toolchain degli strumenti
    - Le versioni di golangci-lint, govulncheck, actionlint e walden stanno solo in `tools.mk`. Gli strumenti si eseguono con `go run modulo@versione` e `GOTOOLCHAIN=go$(GO_VERSION)`, letta da `go.mod`. Il `Makefile` include `tools.mk`, esporta `GOTOOLCHAIN ?= auto` e perde la riga `.PHONY` duplicata (F-45). Il target `tools-check` verifica che golangci-lint sia compilato con la versione di Go di `go.mod` e che ogni strumento riporti la versione di `tools.mk`; stampa `tools-check: ok (go<versione>)`.
    - Requirements: `R6.AC2`, `NFR4`
    - Design: Architecture, Failure Modes And Tradeoffs
    - Verification:
      - command: ["make", "tools-check"]
        expect_output: "tools-check: ok (go"
        covers: ["R6.AC2"]
        timeout: 20m
  - [x] 1.2 Harness di mutazione
    - `scripts/ci/selftest.sh` e target `ci-selftest`, con `SELFTEST_CASES` per scegliere i casi. L'harness:
      - copia in un repository temporaneo l'istantanea del working tree (file tracciati e non tracciati non ignorati), con un commit base;
      - per ogni caso esegue la base, applica la mutazione (su file o su variabili), esegue il target, confronta l'esito e ripristina l'istantanea;
      - stampa `selftest ok: <caso>` oppure `selftest FAIL: <caso> (...)`, e alla fine `selftest summary: N ok, M failed`;
      - esce con errore se un caso fallisce o non esiste.

      `--self-check` verifica l'harness su un repository finto con esiti noti: una mutazione efficace, una inefficace e una base rossa.
    - Requirements: `NFR3`
    - Design: Architecture, Verification Plan
    - Verification:
      - command: ["scripts/ci/selftest.sh", "--self-check"]
        expect_output: "selftest self-check: ok"

- [x] 2. Build, vet, moduli e test
  - [x] 2.1 Target `build-check`
    - TDD: prima i casi `build-proxy-error` (errore di sintassi in `app/app.go`) e `build-plugin-error` (errore in `plugins/hello-plugin/hello_plugin.go`). Poi il target: `go build ./...` e build del plugin con `-buildmode=plugin` in una cartella temporanea; stampa `build-check: ok`.
    - Requirements: `R2.AC1`
    - Design: Architecture
    - Verification:
      - command: ["make", "build-check"]
        expect_output: "build-check: ok"
      - command: ["make", "ci-selftest", "SELFTEST_CASES=build-proxy-error build-plugin-error"]
        expect_output: "selftest summary: 2 ok, 0 failed"
        covers: ["R2.AC1"]
        timeout: 20m
  - [x] 2.2 Target `vet` esteso al plugin
    - TDD: prima il caso `vet-printf` (verbo di formato errato in un `Printf`). Poi il target: `go vet` sul modulo principale e sul plugin; stampa `vet: ok`.
    - Requirements: `R2.AC2`
    - Design: Architecture
    - Verification:
      - command: ["make", "vet"]
        expect_output: "vet: ok"
      - command: ["make", "ci-selftest", "SELFTEST_CASES=vet-printf"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R2.AC2"]
        timeout: 20m
  - [x] 2.3 Target `modules`
    - TDD: prima tre casi:
      - `modules-untidy`: una riga `require` necessaria viene rimossa dal `go.mod` principale;
      - `modules-go-version-skew`: la direttiva `go` del plugin è diversa da quella dell'host;
      - `modules-shared-version-skew`: `yaml.v3` v3.0.0 nell'host, dopo `go mod tidy`, mentre il plugin resta a v3.0.1; messaggio atteso `shared module version mismatch`.

      Poi `scripts/ci/modules-check.sh`: `go mod tidy -diff` su host e plugin; stessa versione di Go in `go.mod`, nel `go.mod` del plugin e in `GOTOOLCHAIN` del `Dockerfile`; stesse versioni per i moduli presenti in entrambi i grafi. Stampa `modules: ok`.
    - Requirements: `R2.AC5`
    - Design: Architecture, Failure Modes And Tradeoffs
    - Verification:
      - command: ["make", "modules"]
        expect_output: "modules: ok"
      - command: ["make", "ci-selftest", "SELFTEST_CASES=modules-untidy modules-go-version-skew modules-shared-version-skew"]
        expect_output: "selftest summary: 3 ok, 0 failed"
        covers: ["R2.AC5"]
        timeout: 20m
  - [x] 2.4 Target `test-race` con ordine casuale
    - TDD: `TestExposeMetricsHandler`, eseguito da solo, oggi fallisce (F-51); lo si corregge registrando una richiesta prima di verificare l'esposizione. Poi i casi `test-failing` (un test rosso aggiunto) e `test-data-race` (un test con una data race). Infine il target: `go test -race -shuffle=on -count=1 ./...`; stampa `test-race: ok`.
    - Requirements: `R2.AC3`, `NFR3`
    - Design: Architecture, Options Considered
    - Verification:
      - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-v", "-run", "^TestExposeMetricsHandler$", "./metrics"]
        expect_output: "--- PASS: TestExposeMetricsHandler"
      - command: ["make", "test-race"]
        expect_output: "test-race: ok"
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=test-failing test-data-race"]
        expect_output: "selftest summary: 2 ok, 0 failed"
        covers: ["R2.AC3"]
        timeout: 30m
  - [x] 2.5 Target `test-hermetic`
    - TDD: prima il caso `hermetic-external-dial`, un test che apre una connessione TCP verso `1.1.1.1:443`. Poi `scripts/ci/hermetic.sh` (macOS: `sandbox-exec`; Linux: `sudo unshare --net`, loopback e `setpriv`) e il target: compilazione dei test fuori dall'isolamento, suite completa dentro, controllo negativo (`curl` verso `192.0.2.1`, exit 7). Stampa `test-hermetic: ok`.
    - Requirements: `R2.AC4`, `NFR3`
    - Design: Architecture, Options Considered
    - Verification:
      - command: ["make", "test-hermetic"]
        expect_output: "test-hermetic: ok"
        covers: ["R2.AC4"]
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=hermetic-external-dial"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R2.AC4"]
        timeout: 20m
  - [x] 2.6 Target `image` e immagini base fissate
    - TDD: prima il caso `image-broken-dockerfile` (`RUN exit 1` nello stage builder). Poi il target (docker o podman, `--platform linux/amd64`, nessun push) e le immagini base del `Dockerfile` con tag esplicito e digest. Stampa `image: ok`. Se non c'è un motore container, il caso fallisce come non eseguito.
    - Requirements: `R2.AC6`
    - Design: Architecture, Failure Modes And Tradeoffs
    - Verification:
      - command: ["make", "image"]
        expect_output: "image: ok"
        timeout: 45m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=image-broken-dockerfile"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R2.AC6"]
        timeout: 45m

- [x] 3. Lint e vulnerabilità
  - [x] 3.1 `.golangci.yml` e target `lint`
    - TDD: prima due casi, entrambi con base `HEAD`:
      - `lint-new-violation`: un file nuovo con un errore non controllato, deve fallire;
      - `lint-legacy-only`: nessuna modifica, deve passare nonostante le segnalazioni preesistenti.

      Poi `.golangci.yml`, i target `lint` (`LINT_BASE`, default `main`; `LINT_MODE=merge-base` oppure `rev`) e `lint-all`, e la correzione di `handlers/handlers.go:546` (QF1008). Stampa `lint: ok`.
    - Requirements: `R3.AC1`, `R3.AC2`
    - Design: Architecture, Options Considered, Failure Modes And Tradeoffs
    - Verification:
      - command: ["make", "lint"]
        expect_output: "lint: ok"
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=lint-new-violation lint-legacy-only"]
        expect_output: "selftest summary: 2 ok, 0 failed"
        covers: ["R3.AC1", "R3.AC2"]
        timeout: 30m
  - [x] 3.2 Target `vuln`
    - TDD: prima il caso `vuln-reachable`, che punta il target al modulo di prova `scripts/ci/testdata/vulnfixture` (`go 1.24.0`, chiama `url.ParseQuery`) e lo scansiona con `VULN_RUN_GOTOOLCHAIN=go1.24.0`, applicato tramite `go run -exec`; messaggio atteso `Your code is affected`. Poi il target: govulncheck su host e plugin (`VULN_DIRS`); stampa `vuln: ok`.
    - Requirements: `R3.AC3`
    - Design: Architecture, Verification Plan
    - Verification:
      - command: ["make", "vuln"]
        expect_output: "vuln: ok"
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=vuln-reachable"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R3.AC3"]
        timeout: 20m

- [x] 4. Plugin verificati end-to-end
  - [x] 4.1 Target `smoke-plugins`
    - TDD: prima i casi `smoke-tampered-signature` (`SMOKE_TAMPER=signature`: il `.so` viene modificato dopo la firma) e `smoke-missing-header` (`SMOKE_EXPECT_HEADER=X-Not-Set`). Poi `scripts/ci/plugin-smoke.sh`:
      - in una cartella temporanea compila proxy, signer e plugin, genera chiavi usa e getta, firma il plugin e scrive una config con l'hash;
      - avvia il proxy su loopback e chiama `^/smoke$`, che punta a `/metrics` del proxy e usa `hello-plugin`;
      - verifica il log di caricamento del plugin e l'header;
      - pulisce e controlla che la cartella non esista più.

      Stampa `smoke-plugins: keys removed` e `smoke-plugins: ok`.
    - Requirements: `R4.AC1`, `R4.AC2`, `R4.AC3`
    - Design: Architecture
    - Verification:
      - command: ["make", "smoke-plugins"]
        expect_output: "smoke-plugins: keys removed"
        covers: ["R4.AC1", "R4.AC2", "R4.AC3"]
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=smoke-tampered-signature smoke-missing-header"]
        expect_output: "selftest summary: 2 ok, 0 failed"
        covers: ["R4.AC1", "R4.AC2"]
        timeout: 20m

- [x] 5. Copertura
  - [x] 5.1 Target `coverage` e soglie
    - TDD: prima tre casi:
      - `coverage-below-floor`: viene rimosso `app/app_test.go`;
      - `coverage-floor-lowered`: la soglia di `dito/app` scende rispetto al commit base;
      - `coverage-package-removed`: vengono eliminati `cmd/plugin-signer` e la sua soglia, e il target deve passare.

      Poi `scripts/ci/coverage-gate.sh` e `scripts/ci/coverage-floors.txt`, con le soglie di oggi arrotondate per difetto. Il target:
      - stampa la tabella `| Package | Coverage | Floor |` con il totale, e la aggiunge a `$GITHUB_STEP_SUMMARY` se definito;
      - confronta le coperture con le soglie, e le soglie con quelle di `COVERAGE_BASE` (default `main`, confronto saltato se la base non ha ancora il file);
      - stampa `coverage: ok`.
    - Requirements: `R5.AC1`, `R5.AC2`, `R5.AC3`
    - Design: Architecture, Failure Modes And Tradeoffs
    - Verification:
      - command: ["make", "coverage"]
        expect_output: "| Package | Coverage | Floor |"
        covers: ["R5.AC1"]
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=coverage-below-floor coverage-floor-lowered coverage-package-removed"]
        expect_output: "selftest summary: 3 ok, 0 failed"
        covers: ["R5.AC2", "R5.AC3"]
        timeout: 30m

- [x] 6. Workflow, Renovate e parità locale
  - [x] 6.1 `ci.yml`, `validate-walden.yml` e target `workflows`
    - TDD: prima quattro casi:
      - `workflows-tag-ref`: un `uses:` con `@v7`;
      - `workflows-write-permission`: `contents: write`;
      - `workflows-hardcoded-tool-version`: una versione di govulncheck scritta in un workflow;
      - `workflows-job-without-make`: un job con `run: go test ./...`.

      Poi:
      - `ci.yml`: trigger su PR e push verso `main`, `schedule` settimanale, `workflow_dispatch`, `permissions: contents: read`, `concurrency`, un job per target su `ubuntu-24.04`, `setup-go` con `go-version-file: go.mod`, `fetch-depth: 0` per lint e copertura, basi `LINT_BASE` e `COVERAGE_BASE` per PR e push;
      - `validate-walden.yml`: azioni fissate per SHA, `go-version-file`, `make spec-validate`;
      - il target `spec-validate` e `scripts/ci/workflows-check.sh` con actionlint (senza shellcheck e pyflakes).

      Stampa `workflows: ok`.
    - Requirements: `R1.AC1`, `R1.AC2`, `R1.AC3`, `R1.AC4`, `R1.AC5`, `R3.AC4`, `R6.AC1`, `R6.AC2`
    - Design: Architecture, Verification Plan
    - Verification:
      - command: ["make", "workflows"]
        expect_output: "workflows: ok"
        covers: ["R1.AC1", "R1.AC2", "R1.AC3", "R1.AC4", "R1.AC5", "R3.AC4", "R6.AC1", "R6.AC2"]
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=workflows-tag-ref workflows-write-permission workflows-hardcoded-tool-version workflows-job-without-make"]
        expect_output: "selftest summary: 4 ok, 0 failed"
        covers: ["R1.AC4", "R1.AC5", "R6.AC1", "R6.AC2"]
        timeout: 20m
      - command: ["make", "spec-validate"]
        expect_output: "spec-validate: ok"
  - [x] 6.2 `renovate.json`
    - TDD: prima il caso `renovate-missing-gomodtidyall`, che rimuove l'opzione `gomodTidyAll`. Poi:
      - `renovate.json`: `config:recommended`, `group:allNonMajor`, `helpers:pinGitHubActionDigests`, `docker:pinDigests`, `postUpdateOptions: ["gomodTidyAll"]`, pianificazione settimanale, `ignorePaths` per `testdata`, custom manager per `tools.mk`;
      - nuove regole in `workflows-check.sh`: JSON valido (`jq`), chiavi attese, immagini del `Dockerfile` con tag e digest.

      Il target `workflows` stampa anche `renovate: ok`.
    - Requirements: `R7.AC1`, `R7.AC2`, `R7.AC3`, `R7.AC4`, `R7.AC5`
    - Design: Architecture, Options Considered
    - Verification:
      - command: ["make", "workflows"]
        expect_output: "renovate: ok"
        covers: ["R7.AC1", "R7.AC2", "R7.AC3", "R7.AC4", "R7.AC5"]
        timeout: 20m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=renovate-missing-gomodtidyall"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R7.AC4"]
        timeout: 20m
  - [x] 6.3 Target aggregato `ci`
    - TDD: prima il caso `ci-stops-at-first-failure`, senza esecuzione di base: con un problema di vet, `make ci` deve fallire, il suo output deve contenere `build-check: ok` e non deve contenere `test-race: ok`. Poi il target, che esegue in ordine `tools-check`, `build-check`, `vet`, `modules`, `test-race`, `test-hermetic`, `lint`, `vuln`, `coverage`, `smoke-plugins` e `workflows`; stampa `ci: ok`.
    - Requirements: `R6.AC3`
    - Design: Architecture
    - Verification:
      - command: ["make", "ci"]
        expect_output: "ci: ok"
        timeout: 45m
      - command: ["make", "ci-selftest", "SELFTEST_CASES=ci-stops-at-first-failure"]
        expect_output: "selftest summary: 1 ok, 0 failed"
        covers: ["R6.AC3"]
        timeout: 30m

- [ ] 7. Documentazione e chiusura
  - [x] 7.1 README, template della PR e PRD
    - Nel README, una sezione CI con:
      - i target `make`, `make ci` e `make ci-selftest`;
      - i controlli obbligatori da attivare nella protezione del branch (`C6`);
      - l'installazione dell'app Renovate;
      - i requisiti della verifica ermetica (macOS `sandbox-exec`, Linux `sudo`);
      - la politica delle soglie di copertura.

      Nel template della PR, la voce `make ci`. Nel PRD, F-51, F-52 e lo stato di S-02. Tutto questo va fatto prima del checkpoint 7.2 (lezione registrata).

      Revisione del 30/09/2026: il PRD `SEPTEMBER-STATE.md`, aggiornato come richiesto nel commit `c18a720`, è stato poi tolto dal repository su richiesta dell'owner ed è ora un documento locale. La proof non lo verifica più.
    - Requirements: `NFR4`
    - Design: Architecture, Failure Modes And Tradeoffs
    - Verification:
      - command: ["grep", "-q", "make ci-selftest", "README.md"]
      - command: ["grep", "-q", "Controlli obbligatori", "README.md"]
      - command: ["grep", "-q", "Renovate", "README.md"]
      - command: ["grep", "-q", "make ci", ".github/pull_request_template.md"]
  - [x] 7.2 Checkpoint locale completo
    - Nessuna modifica: `make ci`, `make image` e l'intero harness di mutazione (24 casi).
    - Requirements: `R2.AC1`, `R2.AC2`, `R2.AC3`, `R2.AC4`, `R2.AC5`, `R2.AC6`, `R3.AC1`, `R3.AC2`, `R3.AC3`, `R4.AC1`, `R4.AC2`, `R4.AC3`, `R5.AC1`, `R5.AC2`, `R5.AC3`, `R6.AC1`, `R6.AC2`, `R6.AC3`
    - Design: Verification Plan
    - Verification:
      - command: ["make", "ci"]
        expect_output: "ci: ok"
        covers: ["R2.AC1", "R2.AC2", "R2.AC3", "R2.AC4", "R2.AC5", "R4.AC1", "R4.AC2", "R4.AC3", "R5.AC1", "R6.AC1", "R6.AC2", "R6.AC3"]
        timeout: 45m
      - command: ["make", "image"]
        expect_output: "image: ok"
        covers: ["R2.AC6"]
        timeout: 45m
      - command: ["make", "ci-selftest"]
        expect_output: "selftest summary: 24 ok, 0 failed"
        covers: ["R2.AC1", "R2.AC2", "R2.AC3", "R2.AC4", "R2.AC5", "R2.AC6", "R3.AC1", "R3.AC2", "R3.AC3", "R4.AC1", "R4.AC2", "R5.AC2", "R5.AC3", "R6.AC3"]
        timeout: 90m
  - [x] 7.3 Esecuzioni su GitHub dopo il push
    - Da completare dopo che l'owner ha pubblicato il branch, aperto la PR e fatto il merge. `scripts/ci/observe-github.sh runs` verifica:
      - che le ultime esecuzioni di `ci.yml` per una PR verso `main` e per un push su `main` siano riuscite;
      - che durino al massimo 15 minuti;
      - che il log contenga `tools-check: ok (go<versione di go.mod>)`.
    - Requirements: `R1.AC1`, `R1.AC2`, `R1.AC3`, `NFR1`
    - Design: Verification Plan, Failure Modes And Tradeoffs
    - Verification:
      - command: ["scripts/ci/observe-github.sh", "runs"]
        expect_output: "observe runs: ok"
        covers: ["R1.AC1", "R1.AC2", "R1.AC3"]
  - [ ] 7.4 Esecuzione settimanale su GitHub
    - Da completare almeno una settimana dopo il merge: `observe-github.sh schedule` verifica un'esecuzione programmata di `ci.yml` riuscita negli ultimi 8 giorni.
    - Requirements: `R3.AC4`
    - Design: Verification Plan
    - Verification:
      - command: ["scripts/ci/observe-github.sh", "schedule"]
        expect_output: "observe schedule: ok"
        covers: ["R3.AC4"]
  - [ ] 7.5 Attività di Renovate
    - Da completare dopo l'installazione dell'app. `observe-github.sh renovate` verifica che esista almeno una PR di Renovate e che le sue esecuzioni di `ci.yml` siano riuscite. Le regole di `R7` sono già dimostrate sulla configurazione dal task 6.2; qui si osserva che il bot è attivo e che le sue PR passano la pipeline.
    - Requirements: `R7.AC1`
    - Design: Verification Plan, Failure Modes And Tradeoffs
    - Verification:
      - command: ["scripts/ci/observe-github.sh", "renovate"]
        expect_output: "observe renovate: ok"
