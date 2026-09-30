---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T07:00:40Z
last_modified: 2026-09-30T07:00:40Z
approved_fingerprint: sha256:3d9aa995ab487513125606401422d095b83b782bd3c3abd58399dbba5acc78aa
source_requirements_approved_at: 2026-09-30T06:43:35Z
source_requirements_fingerprint: sha256:e224fe831b3ea48915297cede6e8c2eb0aa160130318bb30ea99dc4132d14390
---

# Feature Design

## Architecture

La pipeline è un insieme di **target `make`** che la CI e gli sviluppatori eseguono allo stesso modo. I workflow GitHub preparano solo l'ambiente (checkout, toolchain da `go.mod`, cache) e invocano un target per job. La logica sta in pochi script shell sotto `scripts/ci/` (compatibili con la bash 3.2 di macOS), senza nuove dipendenze Go.

### Componenti

| File | Ruolo | Criteri |
| --- | --- | --- |
| `.github/workflows/ci.yml` (nuovo) | Trigger: pull request verso `main`, push su `main`, esecuzione programmata settimanale (lunedì 06:00 UTC), avvio manuale. `permissions: contents: read` a livello di workflow. Un job per controllo su `ubuntu-24.04`, ognuno esegue `make <target>`. `concurrency` annulla le esecuzioni superate della stessa pull request. | `R1`, `R3.AC4` |
| `.github/workflows/validate-walden.yml` (modificato) | Trigger invariati (`C1`); azioni fissate per SHA, toolchain da `go.mod`, validazione tramite `make spec-validate`. | `R1.AC3`, `R1.AC5`, `R6.AC2` |
| `tools.mk` (nuovo, incluso dal `Makefile`) | Unico punto delle versioni degli strumenti: golangci-lint v2.14.0, govulncheck v1.8.0, actionlint v1.7.12, walden v0.11.0. Gli strumenti girano con `go run <modulo>@<versione>`, compilati con la toolchain di `go.mod` (`GOTOOLCHAIN=go$(GO_VERSION)`, con `GO_VERSION` letta da `go.mod`). | `R6.AC2`, `C2`, `C4` |
| `Makefile` (esteso) | Nuovi target (tabella sotto) e `ci` aggregato; `GOTOOLCHAIN ?= auto` esportato, così i target funzionano anche con un Go locale più vecchio. | `R6` |
| `scripts/ci/hermetic.sh` | Esegue un comando con la rete in uscita limitata al loopback. Su macOS usa `sandbox-exec` con il profilo di S-01; su Linux usa `sudo unshare --net`, attiva il loopback e restituisce i privilegi all'utente con `setpriv`. | `R2.AC4` |
| `scripts/ci/modules-check.sh` | Tre controlli: `go mod tidy -diff` per il modulo principale e per il plugin; stessa versione di Go in `go.mod`, nel `go.mod` del plugin e in `GOTOOLCHAIN` del `Dockerfile`; stesse versioni dei moduli condivisi tra host e plugin (confronto di `go list -m all`). | `R2.AC5`, `R1.AC3`, `R4.AC1` |
| `scripts/ci/coverage-gate.sh` e `scripts/ci/coverage-floors.txt` | Misura la copertura di ogni package con i suoi test e stampa la tabella per package e il totale, sia in output sia nel riepilogo del job. Confronta la copertura con le soglie (interi arrotondati per difetto dai valori di oggi) e le soglie con quelle del ramo base: non possono scendere, ma quella di un package eliminato può sparire. | `R5` |
| `scripts/ci/plugin-smoke.sh` | In una cartella temporanea: compila proxy, signer e plugin con la stessa toolchain; genera chiavi usa e getta e firma il plugin; scrive una config con l'hash calcolato; avvia il proxy e chiama una location con `hello-plugin` che punta al `/metrics` del proxy stesso. Verifica che il plugin sia stato caricato (dal log) e che la risposta abbia l'header `X-Hello-Plugin`; alla fine cancella tutto. | `R4` |
| `scripts/ci/workflows-check.sh` | Regole sui workflow: `uses:` solo con SHA di 40 caratteri; permessi di sola lettura; ogni job di `ci.yml` esegue un target `make` esistente; nessuna versione di strumenti fuori da `tools.mk`; trigger e toolchain attesi; chiavi attese in `renovate.json`. In più esegue actionlint. | `R1`, `R6.AC1`, `R6.AC2`, `R7` |
| `scripts/ci/selftest.sh` | Harness di mutazione per le proof locali. Copia il working tree in un repository temporaneo (commit base = istantanea), introduce per ogni gate un difetto del tipo corrispondente, esegue il target e verifica che fallisca. Verifica anche i casi limite che devono passare: lint sul solo debito esistente, soglia di un package eliminato. | criteri IF/THEN |
| `.golangci.yml` (nuovo) | golangci-lint v2 con il set standard (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) più `gosec`, `bodyclose`, `errorlint`, `noctx`, `nilerr`, `copyloopvar`, `forcetypeassert`, `modernize`, `nolintlint`; formatter `gofmt`. Il filtro "solo segnalazioni nuove" lo passa il target (`--new-from-merge-base` o `--new-from-rev`), non la configurazione. | `R3.AC1`, `R3.AC2` |
| `renovate.json` (nuovo) | Renovate (app GitHub) con `config:recommended`, `group:allNonMajor`, `helpers:pinGitHubActionDigests`, `docker:pinDigests`, `postUpdateOptions: ["gomodTidyAll"]`, pianificazione settimanale e un custom manager per le versioni in `tools.mk`. | `R7`, `R1.AC5` |
| `Dockerfile` (modificato) | Immagini base con tag esplicito e digest (oggi il runtime è `ubi-minimal:latest`), così il bot può proporne gli aggiornamenti. | `R7.AC3` |
| `metrics/metrics_test.go` (modificato) | **F-51**: `TestExposeMetricsHandler` registra una richiesta prima di verificare l'esposizione, invece di dipendere da `TestRecordRequest` che oggi gira prima. Serve per `-shuffle=on`. | `R2.AC3`, `NFR3` |
| `handlers/handlers.go` (una riga) | `rli.Header()` al posto di `rli.ResponseWriter.Header()` (staticcheck QF1008). La riga è stata toccata dal branch, quindi senza correzione la prima esecuzione del lint sarebbe rossa. Comportamento invariato. | `R3.AC1` |
| `README.md`, `.github/pull_request_template.md` | Sezione CI: target `make`, controlli da rendere obbligatori (`C6`), installazione di Renovate, requisiti della verifica ermetica, politica delle soglie di copertura. Nel template della PR, la voce `make ci`. | constitution |

### Job e target

| Job (`ci.yml`) | Target `make` | Cosa fa | Criteri |
| --- | --- | --- | --- |
| build | `build-check` | `go build ./...` (proxy, signer) e build del plugin con `-buildmode=plugin` in una cartella temporanea | `R2.AC1` |
| vet | `vet` | `go vet` sul modulo principale e sul plugin | `R2.AC2` |
| modules | `modules` | `scripts/ci/modules-check.sh` | `R2.AC5` |
| test | `test-race` | `go test -race -shuffle=on -count=1 ./...` | `R2.AC3` |
| hermetic | `test-hermetic` | compila i test fuori dall'isolamento, poi esegue `go test -count=1 ./...` e il controllo negativo (`curl` verso `192.0.2.1`, exit 7) dentro `hermetic.sh` | `R2.AC4` |
| lint | `lint` | golangci-lint con `LINT_BASE`: nelle PR merge-base con `origin/<base>`, nei push il commit precedente, in locale `main`. `lint-all` mostra tutte le segnalazioni. | `R3.AC1`, `R3.AC2` |
| vuln | `vuln` | govulncheck sul modulo principale e sul plugin | `R3.AC3`, `R3.AC4` |
| coverage | `coverage` | `scripts/ci/coverage-gate.sh`, con `COVERAGE_BASE` scelta come per il lint | `R5` |
| smoke | `smoke-plugins` | `scripts/ci/plugin-smoke.sh` | `R4` |
| image | `image` | build `linux/amd64` senza push: docker in CI, podman o docker in locale | `R2.AC6` |
| workflows | `workflows` | actionlint (senza integrazioni shellcheck e pyflakes, per avere lo stesso esito ovunque) e `workflows-check.sh` | `R1`, `R6`, `R7` |

`make ci` esegue in ordine `build-check`, `vet`, `modules`, `test-race`, `test-hermetic`, `lint`, `vuln`, `coverage`, `smoke-plugins` e `workflows`, fermandosi al primo errore (`R6.AC3`). `image` resta fuori perché richiede un motore container. `make ci-selftest` esegue l'harness di mutazione.

Due nuovi problemi emersi durante il design, da registrare nel PRD alla chiusura della spec:

- **F-51**: un test dipende dall'ordine di esecuzione. Qui è corretto.
- **F-52**: host e plugin possono risolvere versioni diverse di un modulo condiviso senza che build o `tidy` lo segnalino (verificato con `yaml.v3` v3.0.0 contro v3.0.1). Qui è intercettato dal controllo dei moduli.

## Options Considered

| Decisione | Scelta | Alternativa reale | Motivo |
| --- | --- | --- | --- |
| Bot di aggiornamento | Renovate | Dependabot | Per `R7.AC4` Dependabot richiede `group-by: dependency-name`, che crea una PR per dipendenza ed entra in conflitto con `R7.AC5`. Renovate li soddisfa entrambi con `group:allNonMajor` e `gomodTidyAll`, e aggiorna anche `tools.mk`. Richiede l'installazione dell'app. |
| Versioni degli strumenti | `go run modulo@versione`, con le versioni in `tools.mk` | un modulo `tools/go.mod` con direttive `tool`; un modulo per strumento; `golangci-lint-action` | Un modulo unico mette tutti gli strumenti nello stesso grafo MVS, e golangci-lint è sensibile alla versione di `golang.org/x/tools`. Un modulo per strumento moltiplica i file e le direttive `go` da allineare. L'action tiene la versione nel workflow (un secondo punto) e usa un binario precompilato con una Go che può essere più vecchia di 1.27 (`C4`). |
| Rete limitata su Linux | `sudo unshare --net` con `setpriv` | container con `--network none`; regole iptables; user namespace senza privilegi | Il container richiede un'immagine e le cache montate. Le regole iptables agiscono su tutto il runner, che durante il job deve restare connesso a GitHub. Gli user namespace senza privilegi sono limitati da AppArmor su Ubuntu 24.04. La scelta è verificata in `ubuntu:24.04`: exit 7 verso l'esterno, loopback funzionante, file di proprietà dell'utente. |
| Insieme dei linter | mirato su correttezza e sicurezza | la configurazione completa della skill golang-lint (48 linter) | Con 48 linter, ogni riga legacy toccata da S-03…S-13 farebbe emergere regole di stile. L'insieme si può estendere in seguito. |
| Misura della copertura | per package, con i suoi test | `-coverpkg=./...` (conta anche i test degli altri package) | È più stabile e attribuibile: la soglia di un package dipende solo dai suoi test. Il totale passa da 50,4% a 48,3%. |
| Pubblicazione della copertura | riepilogo del job | Codecov | Nessun servizio esterno e nessun token. |
| Struttura della pipeline | un job per controllo | un solo job con `make ci` | I job in parallelo restano entro `NFR1` e danno uno status check per gate, utile per la protezione del branch (`C6`). |
| Prove dei gate IF/THEN | harness di mutazione in locale | fidarsi della semantica degli strumenti | Senza mutazioni una proof passerebbe anche con un gate rotto, per esempio con un filtro che scarta tutto. |
| Ordine dei test | `-shuffle=on` | ordine fisso | L'ordine fisso ha nascosto F-51; con lo shuffle, il seed stampato rende riproducibile ogni fallimento. |

## Simplicity And Elegance Review

- Una sola implementazione per controllo: il workflow chiama `make`, e il target chiama uno script o uno strumento. Nessuna logica duplicata tra YAML e ambiente locale (`R6.AC1`).
- Nessuna nuova dipendenza nei `go.mod`: gli strumenti girano con `go run` (`C2`).
- Script shell con strumenti standard (`awk`, `git`, `curl`) invece di un programma Go dedicato: meno codice da mantenere e nessun impatto sul modulo.
- Lo smoke usa come backend l'endpoint `/metrics` del proxy stesso, senza un server di test in più.
- Test con race detector e test ermetici restano in job separati: falliscono per cause diverse, e il job ermetico non deve dipendere dal race detector.
- L'harness di mutazione è il pezzo più corposo, ma è l'unico modo per dimostrare in locale i criteri IF/THEN, e resta un solo script.

## Failure Modes And Tradeoffs

- **Proof locali e comportamento su GitHub (`C7`)**: trigger, durata, esecuzione settimanale e PR di Renovate si osservano solo dopo il push. L'ultimo task resta in attesa fino ad allora.
- **Azioni dell'owner**: installare l'app Renovate e rendere obbligatori i controlli nella protezione del branch (`C6`). Il README le documenta.
- **Linux in locale**: `test-hermetic` usa `sudo`, che può chiedere la password; sui runner GitHub `sudo` non la chiede.
- **`sandbox-exec` è deprecato su macOS**: se sparisse, `test-hermetic` in locale andrebbe rivisto; su Linux non cambia nulla.
- **Toolchain degli strumenti**: golangci-lint v2.14.0 e govulncheck v1.8.0 dichiarano `go 1.26.0`. Con `GOTOOLCHAIN=auto` e un Go locale più vecchio verrebbero compilati con 1.26, e golangci-lint rifiuterebbe il modulo 1.27. Per questo `tools.mk` forza la toolchain di `go.mod`. Se uno strumento richiedesse una Go più nuova di `go.mod`, la sua build fallirebbe: è il segnale per un upgrade deliberato.
- **Lint solo sulle novità**: emergono le segnalazioni sulle righe legacy modificate (chi tocca una riga la sistema). Spostare o rinominare un file fa apparire come nuove le sue segnalazioni. Il filtro richiede la storia completa (`fetch-depth: 0`).
- **Soglie di copertura**: sono arrotondate per difetto all'intero, per assorbire piccole variazioni tra esecuzioni. Alzarle è compito delle spec che aggiungono test (politica scritta nel README); il confronto con il ramo base impedisce di abbassarle.
- **Esecuzioni programmate**: GitHub le disattiva dopo 60 giorni senza attività nel repository. Possono diventare rosse per una nuova CVE senza modifiche al codice: è il segnale voluto.
- **Durata**: il job più lento è quello dell'immagine (UBI da circa 1 GB più il download della toolchain), stimato in 3-6 minuti, in parallelo agli altri. La prima compilazione degli strumenti richiede 1-2 minuti; dopo interviene la cache di `setup-go`.
- **Versioni condivise tra host e plugin**: oggi l'unico modulo condiviso è `yaml.v3` (archiviato). Il controllo di coerenza intercetta un disallineamento prima che lo scopra lo smoke.
- **Mutazione dell'immagine**: richiede un motore container. Se non c'è, l'harness segnala la mutazione come non eseguita e fallisce, invece di dare un successo silenzioso.

## Verification Plan

Le proof di Walden girano in locale con `env GOTOOLCHAIN=auto`, come in S-01. Le osservazioni "dopo il push" appartengono a un ultimo task che resta in attesa finché l'owner non pubblica il branch (`C7`); si leggono con `gh run list` e `gh run view`.

| Criteri | Proof locale | Dopo il push |
| --- | --- | --- |
| `R1.AC1`, `R1.AC2`, `R3.AC4` | `make workflows`: trigger `pull_request` e `push` su `main`, `schedule` settimanale | esecuzioni su PR, su `main` e settimanali |
| `R1.AC3` | `make workflows`: `go-version-file: go.mod` in ogni job | log dei job con la versione di `go.mod` |
| `R1.AC4`, `R1.AC5` | `make workflows`; selftest: `uses: …@v7` e `contents: write` devono far fallire il controllo | — |
| `R2.AC1`, `R2.AC2`, `R2.AC3`, `R2.AC5` | `make ci` verde; selftest: errore di compilazione, problema di vet, test rosso, data race, dipendenza non usata, versione di Go disallineata: ogni target fallisce | job verdi sulla prima PR |
| `R2.AC4` | `make test-hermetic`, con controllo negativo | job `hermetic` verde su Linux |
| `R2.AC6` | `make image` con podman; selftest: `Dockerfile` rotto, il target fallisce | job `image` verde |
| `R3.AC1`, `R3.AC2` | selftest: una violazione nuova fa fallire il target; nessuna modifica al codice legacy lo fa passare | — |
| `R3.AC3` | selftest: modulo di prova con toolchain vulnerabile (Go 1.24.0), `vuln` fallisce | — |
| `R4` | `make smoke-plugins` verde; selftest: firma manomessa e header atteso diverso fanno fallire il target | job `smoke` verde |
| `R5` | `make coverage` verde, con la tabella; selftest: test rimosso e soglia abbassata fanno fallire il target; package e soglia eliminati lo fanno passare | tabella nel riepilogo del job |
| `R6.AC1`, `R6.AC2` | `make workflows`: ogni job chiama un target esistente; versioni solo in `tools.mk` | — |
| `R6.AC3` | `make ci` verde; selftest: con un errore di vet, `make ci` fallisce e non esegue i target successivi | — |
| `R7` | `make workflows`: chiavi di `renovate.json` | PR di Renovate dopo l'installazione dell'app |
| `NFR1` | — | durata delle esecuzioni ≤ 15 minuti |

- I target e l'harness non modificano il working tree: output, profili di copertura e repository temporanei vivono fuori dal repository, così le proof restano in sola lettura.
- I test di `scripts/ci/selftest.sh` stanno nello script stesso: ogni mutazione dichiara il target, il difetto e l'esito atteso.

## Requirement Coverage

| Requirement | Covered By |
| --- | --- |
| `R1` | `ci.yml` (trigger, permessi, toolchain, SHA) e `validate-walden.yml` aggiornato; verificati da `make workflows` e, dopo il push, dalle esecuzioni su GitHub |
| `R2` | Target `build-check`, `vet`, `test-race`, `test-hermetic`, `modules`, `image`; mutazioni in `ci-selftest` |
| `R3` | `.golangci.yml` e target `lint` con la base; target `vuln`; `schedule` in `ci.yml`; mutazioni in `ci-selftest` |
| `R4` | `scripts/ci/plugin-smoke.sh` (target `smoke-plugins`) con chiavi usa e getta; mutazioni su firma e header |
| `R5` | `scripts/ci/coverage-gate.sh` e `coverage-floors.txt`; mutazioni su test, soglie e package |
| `R6` | `tools.mk`, un target `make` per job, `make ci`; regole in `workflows-check.sh` |
| `R7` | `renovate.json` (`group:allNonMajor`, `gomodTidyAll`, digest di action e immagini, `tools.mk`); `Dockerfile` con tag e digest; osservazione dopo l'installazione dell'app |
| `NFR1` | Job in parallelo; durata osservata sulle esecuzioni dopo il push |
| `NFR2` | `permissions: contents: read`, azioni fissate per SHA, nessun segreto nei job (smoke con chiavi usa e getta) |
| `NFR3` | Test ermetici, `-shuffle=on` con F-51 corretto, strumenti con versioni fissate |
| `NFR4` | Stessa toolchain (`go.mod`) e stesse versioni (`tools.mk`) in CI e in locale, attraverso gli stessi target |
| `C1` | `validate-walden.yml` mantenuto, aggiornato solo in pin e toolchain |
| `C2` | Strumenti via `go run`, nessuna direttiva `tool` nei `go.mod` |
| `C3` | Runner `ubuntu-24.04` con CGO; immagine `linux/amd64` |
| `C4` | `GOTOOLCHAIN=go$(GO_VERSION)` in `tools.mk` |
| `C5` | Base del lint: merge-base con `origin/<base>` nelle PR, commit precedente nei push |
| `C6` | README: elenco dei controlli da rendere obbligatori |
| `C7` | Verification Plan: proof locali più osservazioni dopo il push |
