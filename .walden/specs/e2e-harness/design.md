---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T20:03:50Z
last_modified: 2026-09-30T20:03:50Z
approved_fingerprint: sha256:b332b26e2edca7b60eb0e5a377a66aeb48c43a3ec64364b2a5a428592ea964bb
source_requirements_approved_at: 2026-09-30T19:45:38Z
source_requirements_fingerprint: sha256:6bc77e96c03f87c057734ade221269021e4a058d3bd64653c999bd0cac033b9f
---

# Feature Design

## Architecture

La suite vive in un package di soli test, `e2e/` (`package e2e`), eseguito da `go test` come ogni altro package: niente build tag, niente API esportate, niente soglia di copertura. L'esperimento del 30/09/2026 conferma che un package di soli test compila, passa `go vet` e compare nella copertura come `[no statements]`, che il gate di S-02 ignora.

Ogni scenario è un normale test Go che chiama `Run` (`R6.AC1`):

```go
func TestResponseBody_R01_LargeWithContentLength(t *testing.T) {
	Run(t, KnownBug("F-01"), func(s *S) {
		s.Backend("big", deterministicBody(200<<10, withContentLength))
		p := s.Proxy(`locations:
  - {path: "^/big$", target_url: "{{backend "big"}}/big", replace_path: true}`)
		r := s.Get(p, "/big")
		s.Equal(200<<10, r.BodyLen)
		s.Equal(deterministicSHA256(200<<10), r.BodySHA256)
	})
}
```

Il nome segue la forma `Test<Area>_<ID>_<Descrizione>` (`R3.AC14`). `-run TestLimits` seleziona un'area e `-run _R05_` uno scenario (`R5.AC2`); `go test -list` produce l'elenco degli scenari.

### File

| File | Contenuto | Requisiti |
|---|---|---|
| `main_test.go` | `TestMain`: esegue i test, controlla che non restino processi figli e stampa il riepilogo | `R1.AC5`, `R4.AC4` |
| `harness_test.go` | `Run`, `S`, `KnownBug`, registro dei risultati, diagnostica | `R4`, `R2.AC8` |
| `proxy_test.go` | proxy in-process: template della config, logger catturato, catena condivisa | `R1.AC1`, `R1.AC2`, `R2.AC7` |
| `binary_test.go` | build una sola volta del proxy (normale e `-race`), del `plugin-signer` e del plugin; avvio, attesa della prontezza, log, arresto | `R1.AC3`, `R1.AC4`, `R1.AC5` |
| `backend_test.go` | backend `httptest` HTTP, HTTPS, mTLS e WebSocket che registrano le richieste ricevute | `R1.AC2`, `R2.AC5` |
| `client_test.go` | osservazioni del client: framing, `1xx`, hash in streaming, tempi di arrivo, interruzioni; client WebSocket | `R2.AC1`…`R2.AC4`, `R2.AC6` |
| `certs_test.go` | CA, certificati server e client generati con `crypto/x509` | `R3.AC9` |
| `harness_selftest_test.go` | test dell'harness stesso, contro backend diretti e con un `TB` finto | `R1.AC4`, `R2`, `R4` |
| `<area>_test.go` | scenari, un file per area | `R3` |

### Catena condivisa (`C2`)

La costruzione della catena esce da `StartServer` in una funzione di `handlers`:

```go
// NewHandler returns the handler that serves every request: access logging
// around the dynamic proxy with the loaded plugins.
func NewHandler(dito *app.Dito, plugins []plugin.Plugin) http.Handler
```

`cmd/main.go` la usa al posto del `mux` costruito sul posto; il comportamento non cambia. `handlers` importerà `middlewares`, che non importa `handlers`: nessun ciclo.

### Proxy in-process

`s.Proxy(yaml, opts...)`:

1. rende il template della config con gli indirizzi dei backend dello scenario (`{{backend "nome"}}`, `{{ws "nome"}}`, `{{ca "nome"}}`, `{{clientCert}}`, `{{clientKey}}`) e lo scrive in una directory temporanea;
2. carica la config con `config.LoadConfiguration`, che applica default e validazione come il binario, e la imposta con `config.UpdateConfig`;
3. crea un logger `slog` che scrive in un buffer letto dallo scenario, crea `app.NewDito` e passa a `handlers.NewHandler` i plugin finti dello scenario (tipi Go che implementano `plugin.Plugin`);
4. serve la catena con `httptest.NewServer` su loopback.

Gli scenari in-process sono sequenziali, per via dello stato globale (`C3`).

### Binario

Il binario serve per ciclo di vita, hot reload, plugin firmati e race durante il reload (`R1.AC3`). Il flusso:

- `go build` di `./cmd`, eseguito una volta per processo di test (`sync.Once`), con `GOTOOLCHAIN=go<versione di go.mod>` e l'output in una directory temporanea. Le varianti con `-race`, il `plugin-signer` e il plugin (`-buildmode=plugin`) seguono lo stesso schema.
- Il processo parte con `-f <config>` in una directory di lavoro temporanea, su una porta libera scelta dall'harness; se il bind fallisce riprova fino a tre volte. Stdout e stderr finiscono nel buffer dei log.
- È pronto quando risponde una qualsiasi richiesta HTTP. Oltre il tempo massimo lo scenario fallisce con i log (`R1.AC4`).
- L'arresto usa `SIGTERM` e poi `SIGKILL` dopo un tempo massimo; `TestMain` verifica che nessun processo avviato dall'harness sia ancora vivo (`R1.AC5`).
- L'hot reload riscrive il file e ne sposta `mtime` avanti di 2 s, così il polling di `WatchConfig` (2 s più 1 s di attesa) lo rileva anche con timestamp a bassa risoluzione.

### Osservazioni

- Il client usa `http.Transport` con compressione disattivata, per vedere i byte esatti. Una risposta osservata contiene:
  - status, header e trailer;
  - `Content-Length` dichiarato e `TransferEncoding`;
  - lunghezza, SHA-256 e primi 4 KiB del body, calcolati in streaming (`R2.AC1`, `R2.AC2`);
  - i `1xx` ricevuti, tramite `httptrace`;
  - l'errore di lettura, per distinguere un'interruzione da una fine regolare (`R2.AC4`).
- Lo streaming usa un lettore con scadenze, che registra quando arriva ogni blocco (`R2.AC3`). Il client WebSocket è `gorilla/websocket` (`R2.AC6`).
- I backend registrano per ogni richiesta metodo, path, query, header, `Host`, framing, lunghezza e hash del body (`R2.AC5`). Il body servito è generato in modo deterministico, per confrontarne l'hash senza tenerlo in memoria.
- I log in-process sono asincroni (canale e worker del middleware):
  - la presenza di un log si verifica aspettando fino a un tempo massimo;
  - l'assenza si verifica dopo un intervallo di assestamento. Un log in ritardo può far passare lo scenario per errore, ma non farlo fallire per errore.
- Le metriche si leggono dall'endpoint `/metrics` con un parser minimo del formato testuale, e si confrontano variazioni, non valori assoluti (`R3.AC11`).

### Bug noti (`R4`)

`S` implementa l'interfaccia `TestingT` di `testify` (`Errorf`, `FailNow`), quindi gli scenari usano `assert` e `require` tramite `s`:

- **Errore di asserzione**: viene registrato. `FailNow` interrompe il corpo con un panic sentinella, recuperato da `Run`.
- **Errore dell'harness** (config non caricata, backend non avviato, proxy non pronto, tempo scaduto, panic non sentinella): chiama subito `t.Fatalf`, anche negli scenari marcati (`R4.AC3`).

Alla fine del corpo, `Run` decide:

| Marcatore | Asserzioni fallite | Esito |
|---|---|---|
| no | no | `PASS` |
| no | sì | `FAIL` con diagnostica: richieste, risposte osservate, richieste viste dai backend, log del proxy (`R2.AC8`) |
| `KnownBug(ids...)` | sì | `SKIP` con `KNOWN BUG F-xx` e la prima asserzione fallita (`R4.AC1`) |
| `KnownBug(ids...)` | no | `FAIL`: "scenario passes: remove KnownBug(F-xx)" (`R4.AC2`) |

Ogni esito entra in un registro di package. Alla fine `TestMain` stampa:

```
e2e summary: 47 passed, 0 failed, 21 known bugs (F-01: 7, F-02: 2, F-54: 5, ...)
```

`Run` dipende da un'interfaccia minima (`Helper`, `Errorf`, `Fatalf`, `Skipf`, `Logf`, `Cleanup`, `Name`), soddisfatta da `*testing.T` e da un `TB` finto. Così i test dell'harness verificano `R4` senza servizi esterni. Il flag `-shuffle` di `go test` mescola l'ordine degli scenari.

### Esecuzione (`R5`)

- `make e2e` esegue `go test -count=1 -race -shuffle=on -v ./e2e/`; con `SCENARIO=<regex>` aggiunge `-run '<regex>'`.
- `test-race`, `test-hermetic` e `coverage` eseguono i package unitari come oggi e poi `./e2e/` con `-v`, in un'invocazione separata. Così gli scenari e il riepilogo compaiono nei log dei tre job (`R5.AC3`) e il gate di copertura continua a leggere solo le righe dei package unitari.
- `test-hermetic` scarica prima, fuori dall'isolamento, anche le dipendenze del modulo del plugin, perché dentro l'isolamento le build dell'harness non hanno rete (`R5.AC4`).
- `make ci-selftest` riceve due casi nuovi (`R5.AC5`, `R6.AC1`):
  - `e2e-regression`: toglie la riscrittura dell'header `Host` in `createDirector` e verifica che `make test-race` fallisca citando `TestHeaders_HostRewritten`. Non si usa la query: `ReverseProxy` clona la richiesta in ingresso, quindi la query sopravvive anche senza l'assegnazione nel `Director`;
  - `e2e-new-scenario`: aggiunge un solo file con uno scenario che fallisce e verifica che la suite lo esegua senza modifiche all'harness.

### Catalogo iniziale (`R3`)

I marcatori riflettono il comportamento misurato su `main` il 30/09/2026. Ogni scenario scritto viene eseguito subito:

- se fallisce senza un marcatore previsto, la causa è un bug dell'harness oppure un finding nuovo, da registrare con un ID nuovo prima di marcarlo;
- se passa con un marcatore previsto, il marcatore va tolto.

| Area | Scenari | Bug noti previsti |
|---|---|---|
| routing | match per regex, nessuna location (`404`), prima location vincente, `replace_path: true`, `R-05` `replace_path: false`, query conservata | F-04 (`R-05`) |
| headers | `R-06` `X-Forwarded-*`, `Host` riscritto, `X-Request-ID` generato e propagato, hop-by-hop rimossi, `additional_headers`, `excluded_headers`, `excluded_headers` senza distinzione di maiuscole, header di sicurezza presenti | F-05 (`R-06`), F-12 |
| request body | JSON integro, `R-02` form, `R-03` query malformata, limite con `Content-Length`, `R-04` limite chunked, body grande sotto il limite | F-02 (`R-02`, `R-03`), F-03 (`R-04`) |
| response body | vuoto, 1 B, `R-01` 200 KiB con `Content-Length`, `P02` 200 KiB chunked, `P07` SSE, `P06` `103` inoltrato, `P06` status finale dopo `103`, `P05` `HEAD` oltre il limite, `304` con `Content-Length` oltre il limite, `P11` backend che si interrompe, `P10` memoria con 64 MiB | F-01 (`R-01`, `P02`, `P07`, `P10`), F-55 (`P05`, `304`), F-56 (`103`) |
| limits | `P03` `Content-Length` oltre il limite, `P12` primo chunk oltre il limite, `P04` superamento dopo l'inizio (mai una risposta troncata che sembra completa), limite di location, limite globale, `P09` JSON con path particolari, warning nel log | F-54 (`P03`, `P12`, location, globale), F-01 (`P04`), F-54 e F-07 (`P09`) |
| errors | backend irraggiungibile (`502`), timeout della risposta (`504`), target non valido (`500`), client che annulla | — |
| plugins | ordine dei middleware, middleware critico mancante (`500`), plugin firmato caricato (binario), plugin manomesso rifiutato all'avvio (binario) | — |
| websocket | upgrade ed echo verso un backend `ws` | — |
| transport | backend HTTPS con `ca_file`, mTLS con `cert_file` e `key_file`, transport della location che prevale su quello globale | — |
| reload (binario) | nuova location servita, `R-10` ritorno al valore precedente, `R-09` richieste concorrenti durante i reload con il binario `-race` | F-24 (`R-10`), F-23 (`R-09`) |
| metrics | endpoint esposto, `R-08` contatori per richiesta | F-28 e F-29 (`R-08`) |
| logging | log di accesso compatto, `R-11` logging disabilitato, `R-12` verbose | F-32 (`R-11`, `R-12`) |
| lifecycle (binario) | `R-13` avvio senza sezione `plugins`, shutdown ordinato con una richiesta in corso, `R-14` risposta da 200 KiB attraverso il binario | F-35 (`R-13`), F-01 (`R-14`) |

Per le aree che una spec futura ridefinirà, gli scenari verificano il contratto documentato oggi: per esempio lo status `413` dell'errore di limite dal README. Quando la spec cambia il contratto, aggiorna i propri scenari. Lo scenario `P04` verifica invece solo ciò che non dipende dalle decisioni aperte di S-03: il client non deve mai ricevere come completa una risposta troncata.

### Finding nuovi

Le sonde di S-03 (30/09/2026) hanno mostrato tre comportamenti che non corrispondono a nessun finding registrato. Ricevono un ID ora, perché il catalogo li marca, e vanno registrati nel PRD locale:

- `F-54` 🟠: l'errore di limite documentato non arriva al client. La connessione si chiude senza risposta (P3), oppure arriva un `200` con il JSON dell'errore come body (P12).
- `F-55` 🟡: `HEAD` e `304` con un `Content-Length` oltre il limite ricevono `413`, anche se non hanno body (P5).
- `F-56` 🔵: le risposte informative `1xx`, come `103 Early Hints`, non vengono inoltrate (P6, X-01).

## Options Considered

1. **Forma degli scenari.** Scelta: test Go nativi con un wrapper `Run`. Scartate:
   - un catalogo dichiarativo (tabelle o file YAML interpretati): non esprime streaming, tempi, WebSocket e interruzioni senza un interprete che crescerebbe quanto il codice;
   - strumenti esterni (Hurl, venom, k6): sono nuove dipendenze, vietate da `C1`.
2. **Modalità di esecuzione.** Scelta E1: in-process con la catena condivisa, più il binario per il ciclo di vita. Scartati:
   - solo binario: più lento, e il race detector non vedrebbe il codice del proxy negli scenari comuni;
   - solo in-process: non copre `main.go`, cioè plugin, watcher e segnali.
3. **Bug noti.** Scelta E2: fallimenti attesi rigorosi, raccolti con un `TestingT` che registra. Scartati:
   - `t.Skip` con un commento: non si accorge di quando il bug viene corretto;
   - build tag per finding: lo scenario scomparirebbe dalla CI finché resta rotto.
4. **Layout.** Scelta: un package di soli test. Scartato un package `internal/e2e` importabile: non serve, perché gli scenari vivono tutti in `e2e/`, e richiederebbe una soglia di copertura e un'API esportata da mantenere.
5. **Visibilità nella CI.** Scelta: un'invocazione separata con `-v` per `./e2e/`. Scartati:
   - `-v` su tutto `./...`: log enormi e un formato che il gate di copertura dovrebbe reinterpretare;
   - un job dedicato: contraddice E4.

## Simplicity And Elegance Review

- Nessun DSL: uno scenario è un test Go, con i nomi e i filtri di `go test`. L'harness aggiunge solo ciò che `httptest` e `testing` non danno: template della config, osservazioni, binario, bug noti e riepilogo.
- L'unica modifica di produzione è `handlers.NewHandler`, e rimuove anche una duplicazione tra `main.go` e i test che assemblavano la catena a mano.
- Le asserzioni riusano `testify`, già dipendenza, invece di un sistema di asserzioni nuovo.
- Il binario si compila una volta per processo, e gli scenari che non ne hanno bisogno non lo toccano.
- Prima prova di semplificazione: rinunciare al binario. Non si può, perché hot reload, segnali e plugin firmati vivono in `main.go`. Il binario resta quindi limitato alle aree reload, lifecycle e plugin firmati.

## Failure Modes And Tradeoffs

- **Stato globale (`C3`)**: scenari in-process sequenziali. Le metriche si confrontano per differenza e la config globale viene reimpostata da ogni scenario. È accettabile perché lo stato dura quanto lo scenario.
- **Log asincroni**: l'assenza di un log si verifica dopo un intervallo di assestamento. Il rischio è un falso positivo (lo scenario passa per errore), mai un falso fallimento.
- **Race note**: in-process una race fa fallire l'intero binario di test anche se attesa. `R-09` gira quindi sul binario `-race` e legge `WARNING: DATA RACE` dal suo output. Ripete i reload fino a 3 volte o fino alla prima race, per evitare un bug noto che a volte passa.
- **Bug noti non deterministici**: un bug marcato deve fallire sempre, altrimenti la regola di `R4.AC2` rende la suite instabile. Ogni scenario marcato deve riprodurre il bug in modo deterministico, e il controllo di stabilità (`NFR3`) lo verifica.
- **Latenza dell'hot reload**: il polling a 2 s più 1 s di attesa rende ogni reload lento. Il budget della suite ne tiene conto, e il riepilogo stampa la durata per area.
- **Isolamento dalla rete**: dentro `test-hermetic` le build del binario e del plugin sono offline. Il modulo del plugin va scaricato prima, e la toolchain fissata a quella di `go.mod` è già in cache.
- **Porte**: tra la scelta di una porta libera e il bind del processo figlio può esserci una collisione. L'harness riprova fino a tre volte.
- **Copertura**: gli scenari non alzano la copertura dei package unitari, perché il gate misura ogni package con i propri test. È un tradeoff accettato: `-coverpkg` cambierebbe la semantica delle soglie di S-02.
- **Contratti che cambieranno**: gli scenari descrivono il contratto documentato oggi, e le spec che lo cambiano aggiornano i propri scenari (per esempio S-03 con D-01).
- **Sicurezza (`C6`)**: gli scenari dei finding di sicurezza aperti restano fuori. Il catalogo quindi non è completo su quell'area finché le relative spec non li aggiungono.

## Verification Plan

- **Test dell'harness** (`harness_selftest_test.go`), eseguiti dalle proof dei task con selettori ancorati e `-v`:
  - confronto tra modalità in-process e binario (`R1.AC1`);
  - proxy che non parte, con i log nell'errore entro il tempo massimo (`R1.AC4`);
  - osservazioni dirette contro i backend: hash di 64 MiB in streaming, framing, `103`, tempi dello streaming, interruzione, richieste registrate, echo WebSocket, lettura dei log (`R2.AC1`…`R2.AC7`);
  - diagnostica di un fallimento provocato (`R2.AC8`);
  - con un `TB` finto: esiti dei bug noti, bug corretto, errore dell'harness in uno scenario marcato e riepilogo (`R4.AC1`…`R4.AC4`).
- **Catalogo**: per ogni area, un task esegue gli scenari (`go test -run '^TestArea'`) e controlla con `go test -list` che esistano i casi richiesti da `R3`. Il controllo finale verifica gli ID `R-01`…`R-14` tranne `R-07`, e le sonde citate (`R3.AC14`).
- **Pulizia**: `TestMain` fallisce se resta un processo figlio; una seconda esecuzione con `-count=2` passa (`R1.AC5`).
- **Esecuzione**:
  - `make e2e` stampa il riepilogo;
  - `make e2e SCENARIO=TestLimits` esegue solo quell'area;
  - `make test-race` e `make test-hermetic` passano;
  - `make ci-selftest` passa con i due casi nuovi (`R5.AC1`, `R5.AC2`, `R5.AC4`, `R5.AC5`, `R6.AC1`).
- **CI dopo il push** (`R5.AC3`): i log dei job `test`, `hermetic` e `coverage` della PR contengono il riepilogo della suite. Si osserva come per S-02, con un modo `e2e` di `scripts/ci/observe-github.sh`.
- **Durata** (`NFR2`): la proof misura `make e2e` ed esige al più 120 s.
- **Stabilità** (`NFR3`): `go test -count=10 -shuffle=on ./e2e/` passa al checkpoint finale, con un timeout dedicato.
- **README** (`R6.AC2`): la sezione sugli scenari contiene un esempio e le istruzioni sui bug noti.

## Requirement Coverage

| Requirement | Covered By |
| --- | --- |
| `R1` | `proxy_test.go` e `binary_test.go`, `handlers.NewHandler`; test dell'harness su confronto delle modalità, avvio fallito e pulizia |
| `R2` | `client_test.go`, `backend_test.go`, log catturati, diagnostica di `Run`; test dell'harness sulle osservazioni dirette |
| `R3` | file `<area>_test.go` del catalogo; esecuzione per area e controllo degli ID con `go test -list` |
| `R4` | `Run`, `KnownBug`, registro e riepilogo in `TestMain`; test dell'harness con un `TB` finto |
| `R5` | target `make e2e`, invocazioni con `-v` in `test-race`, `test-hermetic` e `coverage`, scaricamento del modulo del plugin, casi `e2e-regression` di `make ci-selftest` e modo `e2e` di `observe-github.sh` |
| `R6` | scenari come test Go in un solo file, caso `e2e-new-scenario`, sezione del README |
| `NFR1` | backend e proxy su loopback; suite eseguita da `make test-hermetic` |
| `NFR2` | proof con la durata di `make e2e`; esecuzioni CI entro i 15 minuti di S-02 |
| `NFR3` | `go test -count=10 -shuffle=on ./e2e/` al checkpoint finale; scenari marcati deterministici |
| `NFR4` | diagnostica di `Run`, verificata dal test dell'harness su un fallimento provocato |
| `C1` | solo libreria standard, `gorilla/websocket` e `testify`; `modules` di S-02 lo verifica su `go.mod` |
| `C2` | `handlers.NewHandler` usato da `cmd/main.go`; il confronto delle modalità ne mostra l'equivalenza |
| `C3` | scenari in-process sequenziali e metriche per differenza |
| `C4` | build del plugin e del binario con la stessa toolchain e CGO, solo negli scenari che le richiedono |
| `C5` | invocazioni aggiunte ai target di S-02 e casi nuovi di `make ci-selftest`; nessun controllo esistente viene indebolito |
| `C6` | catalogo senza `R-07`, `wss` con CA personalizzata e middleware non critici mancanti; li aggiungono S-05 e S-08 |
