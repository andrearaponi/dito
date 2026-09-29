---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-29T14:40:23Z
last_modified: 2026-09-29T14:40:23Z
approved_fingerprint: sha256:b32171965b631dee0efc7dd2c80fe76868010b5fa06fa8838cfd4a2f0ab1cbf5
source_requirements_approved_at: 2026-09-29T14:34:19Z
source_requirements_fingerprint: sha256:82c14fae0d2e8752f8b920123af75cdd536a4fdc83274aa1bbdff8e13ec6d7f6
---

# Feature Design

## Architecture

La spec non introduce componenti nuovi. Cambia una riga di codice di produzione e tre file di test; il resto è la configurazione delle verifiche.

| File | Modifica | Criteri |
| --- | --- | --- |
| `writer/limited_buffer.go` | `Read` prende il lock esclusivo (`Lock`) invece di quello condiviso (`RLock`): `bytes.Buffer.Read` modifica l'offset interno, quindi non è un'operazione di sola lettura. Gli altri metodi restano invariati: i metodi che modificano (`Write`, `Reset`, `WriteTo`, `Truncate`, `Grow`, `ReadFrom`) usano già `Lock`, quelli di sola lettura (`String`, `Bytes`, `Len`, `Available`, `IsOverflow`, `TotalSize`, `Clone`) restano su `RLock`. | `R1.AC1`, `R1.AC2` |
| `writer/limited_buffer_test.go` | `TestLimitedBuffer_ConcurrentAccess` resta invariato (oggi fallisce con `-race`). Nuovo `TestLimitedBuffer_ConcurrentReadsExactlyOnce`: scrive 32.768 byte (ogni valore da 0 a 255 ripetuto 128 volte), 8 goroutine leggono in parallelo a blocchi di 7 byte fino a `io.EOF`, poi l'istogramma dei byte letti viene confrontato con quello dei byte scritti. | `R1.AC1`, `R1.AC2` |
| `writer/writer_test.go` | Rimosso `TestConcurrentWrites`, che usa il `ResponseWriter` da più goroutine fuori dal contratto (`C1`). Il conteggio dei byte su più scritture sequenziali resta coperto da `TestStreamingMode` (5 scritture, 600 KB). | `R2.AC1` |
| `config/config_test.go` | `TestWatchConfig`: la callback invia la nuova config su un canale bufferizzato con invio non bloccante. Il test attende sul canale con un timeout di 10 s (invece di dormire 3 s e leggere un `bool` condiviso) e verifica anche il contenuto (`port` = `"9090"`). L'attesa iniziale di 3 s resta: senza un watcher osservabile non si sa quando termina il suo primo controllo (S-06). | `R2.AC1` |
| `handlers/handlers_test.go` | `TestDynamicProxyHandler`: il target diventa un `httptest.NewServer` su loopback che registra le richieste ricevute. Il test verifica status 200, una sola richiesta ricevuta, metodo `GET` e path `/` (la location letterale `/test` senza `replace_path` produce `/`). `setupTestConfig` riceve l'URL del backend come parametro. | `R3.AC2`, `R3.AC1` |

**Toolchain delle verifiche (`C4`)**: le proof eseguono `env GOTOOLCHAIN=auto go …`. Il `go` locale, anche 1.24 con `GOTOOLCHAIN=local` nel GOENV, passa alla versione richiesta da `go.mod` (oggi 1.27.1), che resta l'unica fonte di verità.

**Rete limitata al loopback (`C5`)**: su macOS le proof eseguono `go test` dentro `sandbox-exec` con questo profilo:

```
(version 1)(allow default)(deny network-outbound)
(allow network-outbound (remote ip "localhost:*"))
(allow network-outbound (remote unix-socket))
```

- I socket unix restano permessi: sono comunicazione locale tra processi (per esempio il resolver di sistema), non rete.
- Ascolto e connessione su loopback funzionano dentro la sandbox (verificato).
- Prima dell'esecuzione in sandbox, un passo senza restrizioni compila i test (`go test -run '^$' ./...`) per popolare toolchain, moduli e build cache: l'esecuzione in sandbox non deve scaricare nulla.
- **Controllo negativo**: nella stessa sandbox, `curl` verso `http://192.0.2.1/` (TEST-NET-1, RFC 5737) deve fallire subito con exit 7. Se il blocco non fosse attivo, la connessione andrebbe in timeout con exit 28. Entrambi i comportamenti sono stati verificati.
- L'equivalente per la CI Linux (container con `--network none` oppure `unshare -n`) è rinviato a S-02.

## Options Considered

| Decisione | Scelta | Alternativa reale | Motivo |
| --- | --- | --- | --- |
| Race in `LimitedBuffer.Read` | lock esclusivo solo in `Read` | `sync.Mutex` al posto di `sync.RWMutex` per tutti i metodi | L'alternativa impedirebbe di classificare male un metodo in futuro, ma riscrive tutti i metodi di un tipo che S-03 potrebbe eliminare; la scelta tocca una riga. |
| `TestConcurrentWrites` | rimozione | riscriverlo con scritture sequenziali, oppure rendere il writer thread-safe | La versione sequenziale duplicherebbe `TestStreamingMode`; un writer thread-safe contraddice `C1` e aggiunge lock sul percorso critico delle risposte. |
| Race in `TestWatchConfig` | canale bufferizzato con timeout | `atomic.Bool` con le stesse attese fisse, oppure `testing/synctest` | `atomic.Bool` elimina la race ma lascia l'esito legato a una finestra di circa 1 s; `synctest` richiede un watcher cancellabile (S-06). |
| Backend del test degli handler | `httptest.NewServer` su loopback | uno stub del transport in memoria, oppure saltare il test quando non c'è rete | Lo stub richiederebbe di modificare `TransportCache` (API esportata, `C2`); saltare il test nasconderebbe proprio la dipendenza dalla rete. |
| Verifica ermetica | restrizione di rete del sistema operativo durante la proof | controllo statico degli URL nei test, oppure un guard in `TestMain` | Il controllo statico dà falsi positivi (config come `http://backend`) e non vede le connessioni indirette; un guard su `http.DefaultTransport` non copre i transport di Dito. |
| Toolchain delle proof | `GOTOOLCHAIN=auto` | aggiornare il Go locale, oppure fissare `GOTOOLCHAIN=go1.27.1` | `auto` segue `go.mod` su qualunque macchina; una versione fissa nelle proof andrebbe aggiornata a ogni upgrade di Go. |

## Simplicity And Elegance Review

- Cambia una sola riga di codice di produzione; il resto riguarda i test. Nessuna nuova dipendenza e nessuna API esportata toccata (`C2`, `C3`).
- La verifica ermetica non richiede codice di supporto nel repository: la restrizione sta nella proof, copre ogni percorso di rete (transport, WebSocket, connessioni dirette) e non accoppia i test a un meccanismo specifico.
- Non serve un helper condiviso per i backend `httptest`, perché per ora c'è un solo test che ne ha bisogno. Potrà nascere in S-03 e S-04, dove gli scenari `R-xx` del PRD ne useranno molti.

## Failure Modes And Tradeoffs

- **`TestWatchConfig` dipende ancora dal tempo reale.** Se il primo controllo del watcher (circa 2 s dopo l'avvio) avviene dopo la modifica del file, la callback non viene mai chiamata e il test fallisce per timeout. Il margine attuale è di circa 1 s; la soluzione definitiva è in S-06. Il test dura ancora 5-6 s.
- **La goroutine di `WatchConfig` resta attiva dopo il test** (S-06). Grazie all'invio non bloccante sul canale non può bloccarsi né generare race.
- **Il race detector è probabilistico**: l'assenza di segnalazioni non dimostra l'assenza di race. Per questo `R1.AC2` ha anche un'asserzione funzionale (l'istogramma dei byte).
- **`sandbox-exec` è deprecato da Apple** ma funziona su macOS 26.5. Se sparisse, la proof di `R3.AC1` andrebbe rivista; le probe d'ambiente ne segnalano la disponibilità. Le proof con sandbox funzionano solo su macOS.
- **Cache fredde**: l'esecuzione in sandbox fallirebbe per un motivo d'ambiente, non di codice; il passo di preparazione fuori sandbox lo evita. La prima esecuzione con `GOTOOLCHAIN=auto` può dover scaricare la toolchain, quindi serve rete fuori dalla sandbox.
- **Macchina offline**: il controllo negativo passa comunque (exit 7 per assenza di route). È accettabile, perché in quel caso il traffico esterno è comunque assente.
- **CGO**: su Linux il race detector richiede CGO. Riguarda la CI (S-02), non le proof locali.
- **Fuori scope**: F-50 (`Len` e `Available` dopo `Read`) resta aperto e passa a S-03.

## Verification Plan

Tutte le proof usano `env GOTOOLCHAIN=auto`. La colonna "Rosso oggi" indica cosa succede sul codice attuale e serve a dimostrare che la proof non passa a vuoto. In sviluppo si segue il TDD: i test nuovi o modificati si vedono fallire prima del fix.

| Criterio | Verifica | Rosso oggi |
| --- | --- | --- |
| `R1.AC1` | `TestLimitedBuffer_ConcurrentAccess` con `-race`, selettore ancorato, `-v` e marker `--- PASS` | sì: race e panic |
| `R1.AC2` | Nuovo `TestLimitedBuffer_ConcurrentReadsExactlyOnce` con `-race`: istogramma dei byte letti uguale a quello dei byte scritti | sì: il test non esiste ancora; sul codice attuale ci si aspetta una race, da osservare nel passo TDD |
| `R2.AC1` | `go test -race -count=1 ./...`: exit 0, e con `-race` ogni segnalazione fa fallire il pacchetto. Un marker nell'output conferma che i test sono stati eseguiti. | sì: 3 test falliscono |
| `R3.AC2` | `TestDynamicProxyHandler` dentro la sandbox di rete, selettore ancorato e marker `--- PASS`. Il test verifica metodo, path e numero di richieste ricevute dal backend. | sì: senza rete il test fallisce |
| `R3.AC1` | 1) compilazione dei test fuori sandbox per popolare le cache; 2) `go test -count=1 ./...` dentro la sandbox: exit 0; 3) controllo negativo: `curl` verso `192.0.2.1` nella stessa sandbox, exit 7 | sì: `handlers` fallisce |
| `C3` | `go mod tidy -diff` senza differenze | — |
| `C2` | review del diff: nessuna firma esportata cambia | — |

- **Probe d'ambiente** in `.walden/environment.md` (da creare con i task): `go version` (toolchain locale), `env GOTOOLCHAIN=auto go version` (toolchain effettiva), disponibilità di `sandbox-exec`.
- **Controllo di stabilità** durante l'esecuzione, una sola volta e non come proof: `go test -race -count=3` su `writer`, `config` e `handlers`, per intercettare test instabili.

## Requirement Coverage

| Requirement | Covered By |
| --- | --- |
| `R1` | `Read` con lock esclusivo in `writer/limited_buffer.go`; verificato da `TestLimitedBuffer_ConcurrentAccess` (`R1.AC1`) e dal nuovo `TestLimitedBuffer_ConcurrentReadsExactlyOnce` (`R1.AC2`), entrambi con `-race` |
| `R2` | Fix di `LimitedBuffer.Read`, rimozione di `TestConcurrentWrites`, sincronizzazione di `TestWatchConfig`; verificato dall'esecuzione completa con `-race` (`R2.AC1`) |
| `R3` | Backend `httptest` su loopback in `TestDynamicProxyHandler` (`R3.AC2`); esecuzione completa nella sandbox di rete con controllo negativo (`R3.AC1`) |
| `NFR1` | Le verifiche di `R3.AC1` e `R3.AC2`: la suite passa senza rete esterna, e il controllo negativo dimostra che la rete era davvero bloccata |
| `NFR2` | La suite è verde con `-race` (`R2.AC1`) e le race di `LimitedBuffer` sono escluse anche con un'asserzione funzionale (`R1.AC2`) |
| `C1` | Rimozione di `TestConcurrentWrites`; nessun lock aggiunto a `writer.ResponseWriter` |
| `C2` | Nessuna firma esportata modificata; controllo in review |
| `C3` | `go mod tidy -diff` senza differenze |
| `C4` | `GOTOOLCHAIN=auto` in tutte le proof; probe d'ambiente sulla toolchain effettiva |
| `C5` | `sandbox-exec` su macOS; equivalente Linux rinviato a S-02 |
