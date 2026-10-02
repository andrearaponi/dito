---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T19:45:38Z
last_modified: 2026-09-30T19:45:38Z
approved_fingerprint: sha256:6bc77e96c03f87c057734ade221269021e4a058d3bd64653c999bd0cac033b9f
---

# Requirements Document

## Introduction

La suite di test di Dito non esercita quasi nessuno scenario del proxy:

- un solo test end-to-end automatico, `make smoke-plugins`, che controlla lo status e l'header del plugin ma non il body;
- un solo test con un backend HTTP reale per il package `handlers`;
- copertura 0% per middleware, plugin, websocket e `cmd`.

Per questo la CI è verde anche se `main` tronca le risposte (F-01), rompe i POST con form (F-02) e permette il bypass dell'autenticazione sulle route WebSocket (F-13). Gli harness usati durante l'assessment e le sonde P1-P12 di S-03 erano temporanei e sono stati cancellati.

Questa spec introduce un **harness end-to-end riusabile in Go** e un **catalogo di scenari** che copre tutte le funzionalità del proxy documentate nel README e tutti gli scenari di riproduzione dell'assessment. I comportamenti rotti da finding ancora aperti restano nel catalogo come **bug noti**: la suite li riporta senza fallire, e fallisce quando uno di essi viene corretto senza aggiornare il catalogo. Ogni spec dell'ondata 1 e successive userà l'harness per le proprie proof e renderà verdi i propri scenari.

Fonte: SEPTEMBER-STATE (documento locale) scenari R-01…R-14, indicatore K-7 ("scenari R-01…R-14 trasformati in test di regressione verdi"); sonde P1-P12 della bozza di S-03 `response-body-integrity`; funzionalità del README. Spec candidata S-15, fondamenta come S-01 e S-02.

Decisioni dell'owner (30/09/2026):

- **E1**: la maggior parte degli scenari usa la catena di handler in-process, costruita dallo stesso codice di `cmd/main.go`; gli scenari di ciclo di vita usano il binario;
- **E2**: i comportamenti rotti restano nel catalogo come bug noti, con le regole di `R4`;
- **E3**: gli scenari che dimostrano finding di sicurezza ancora aperti entrano con le rispettive correzioni (`C6`);
- **E4**: la suite gira dentro `go test ./...`, quindi nei controlli esistenti della CI; `make e2e` la esegue da sola.

## Requirements

### R1 Avvio del proxy nell'harness

**User Story:** Come sviluppatore di Dito, voglio avviare il proxy in un test con una configurazione e dei backend dichiarati nello scenario, così da verificarne il comportamento come lo vede un client reale.

#### Acceptance Criteria

1. `R1.AC1` WHEN uno scenario avvia il proxy con una configurazione, the harness SHALL servirlo su un indirizzo loopback con la catena di handler costruita dallo stesso codice del binario.
   - Acceptance check: uno scenario di controllo dà lo stesso esito eseguito sulla catena in-process e sul binario compilato.
2. `R1.AC2` WHEN uno scenario dichiara dei backend, the harness SHALL avviarli su loopback e inserirne gli indirizzi nella configurazione del proxy.
   - Acceptance check: le configurazioni degli scenari non contengono indirizzi o porte fissi; ogni riferimento a un backend viene sostituito con l'indirizzo reale del backend avviato.
3. `R1.AC3` WHERE uno scenario verifica il ciclo di vita del processo (configurazione da file, hot reload, segnali, plugin firmati), the harness SHALL eseguire il binario compilato dal modulo con la toolchain di `go.mod`.
   - Acceptance check: gli scenari di ciclo di vita avviano un processo figlio compilato nella stessa esecuzione dei test e ne osservano l'uscita.
4. `R1.AC4` IF il proxy non è pronto entro il tempo massimo dello scenario, THEN the harness SHALL far fallire lo scenario riportando i log del proxy.
   - Acceptance check: con una configurazione che impedisce l'avvio, lo scenario fallisce entro il tempo massimo e l'output contiene i log del proxy.
5. `R1.AC5` WHEN uno scenario termina, the harness SHALL fermare proxy e backend liberandone porte, goroutine e processi.
   - Acceptance check: dopo la suite non restano processi figli attivi e un'esecuzione ripetuta nello stesso processo di test non trova porte o risorse occupate.

### R2 Client e osservazioni

**User Story:** Come autore di uno scenario, voglio osservare tutto ciò che client e backend vedono, così da scrivere verifiche precise sul contratto del proxy.

#### Acceptance Criteria

1. `R2.AC1` The harness SHALL permettere di verificare status, header, trailer e body delle risposte, con lunghezza e hash per i body grandi.
   - Acceptance check: uno scenario confronta un body da 64 MiB tramite lunghezza e hash senza tenerlo tutto in memoria.
2. `R2.AC2` The harness SHALL permettere di osservare il framing della risposta (`Content-Length` o chunked) e le risposte informative `1xx`.
   - Acceptance check: uno scenario distingue una risposta con `Content-Length` da una chunked e registra un `103` ricevuto prima della risposta finale.
3. `R2.AC3` The harness SHALL permettere di osservare quando arrivano al client i dati inviati in streaming dal backend.
   - Acceptance check: uno scenario SSE verifica che il primo evento arrivi mentre il backend attende ancora prima di inviare il secondo.
4. `R2.AC4` The harness SHALL distinguere una connessione interrotta da una risposta completa.
   - Acceptance check: uno scenario con un backend che chiude a metà body osserva un errore di lettura e non una fine regolare.
5. `R2.AC5` The harness SHALL permettere di osservare le richieste ricevute dal backend: metodo, path, query, header e body.
   - Acceptance check: uno scenario verifica il path e gli header `X-Forwarded-*` visti dal backend.
6. `R2.AC6` The harness SHALL offrire un client WebSocket per l'upgrade e lo scambio di messaggi attraverso il proxy.
   - Acceptance check: uno scenario apre una connessione WebSocket attraverso il proxy e riceve l'echo di un messaggio.
7. `R2.AC7` The harness SHALL permettere di leggere i log prodotti dal proxy durante lo scenario.
   - Acceptance check: uno scenario verifica la presenza o l'assenza di un log di accesso.
8. `R2.AC8` WHEN uno scenario fallisce, the harness SHALL riportare la richiesta, la risposta osservata e i log del proxy.
   - Acceptance check: un fallimento provocato di proposito produce un output con i tre elementi.

### R3 Catalogo degli scenari

**User Story:** Come maintainer, voglio che ogni funzionalità del proxy abbia scenari end-to-end, così che nessuna regressione passi inosservata e i bug noti siano visibili finché non vengono corretti.

#### Acceptance Criteria

1. `R3.AC1` The suite SHALL includere scenari di routing: match per regex, `replace_path` vero e falso, conservazione della query, nessuna location corrispondente e ordine delle location.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-05` è tra questi.
2. `R3.AC2` The suite SHALL includere scenari sugli header: `X-Forwarded-*`, `Host`, `X-Request-ID`, header hop-by-hop, `additional_headers`, `excluded_headers` e header di sicurezza nelle risposte.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-06` è tra questi.
3. `R3.AC3` The suite SHALL includere scenari sul body delle richieste: JSON, form, query malformate e limite di dimensione con e senza `Content-Length`.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-02`, `R-03` e `R-04` sono tra questi.
4. `R3.AC4` The suite SHALL includere scenari sul body delle risposte: dimensioni diverse con e senza `Content-Length`, streaming SSE, risposte `1xx`, `HEAD` e `304`, backend che si interrompe.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-01` e le sonde P1, P2, P5, P6, P7, P10 e P11 sono tra questi.
5. `R3.AC5` The suite SHALL includere scenari sui limiti di risposta: `Content-Length` oltre il limite, superamento durante lo streaming, limite di location e limite globale, formato dell'errore.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; le sonde P3, P4, P9 e P12 sono tra questi.
6. `R3.AC6` The suite SHALL includere scenari sugli errori verso il backend: backend irraggiungibile, timeout della risposta, target non valido e client che annulla la richiesta.
   - Acceptance check: ogni caso elencato ha almeno uno scenario con status e formato dell'errore attesi.
7. `R3.AC7` The suite SHALL includere scenari su plugin e middleware: applicazione nell'ordine configurato, middleware critico mancante, plugin firmato caricato e plugin manomesso rifiutato.
   - Acceptance check: ogni caso elencato ha almeno uno scenario; i due casi sui plugin firmati usano il binario e un plugin compilato nella stessa esecuzione.
8. `R3.AC8` The suite SHALL includere scenari WebSocket: upgrade ed echo attraverso il proxy verso un backend `ws`.
   - Acceptance check: almeno uno scenario apre la connessione attraverso il proxy e riceve l'echo; i casi di sicurezza restano fuori (`C6`).
9. `R3.AC9` The suite SHALL includere scenari su transport e TLS verso i backend: backend HTTPS con CA personalizzata, mTLS con certificato client e timeout del transport.
   - Acceptance check: ogni caso elencato ha almeno uno scenario, con certificati generati durante il test.
10. `R3.AC10` The suite SHALL includere scenari di hot reload: nuova location servita dopo la modifica del file, ritorno a un valore precedente e richieste concorrenti durante i reload.
    - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-09` e `R-10` sono tra questi. `R-09` esegue il binario compilato con il race detector e tratta come fallimento dello scenario un rapporto di race nel suo output, perché in-process una race farebbe fallire l'intero binario di test anche se attesa.
11. `R3.AC11` The suite SHALL includere scenari sulle metriche: endpoint esposto e contatori per richiesta.
    - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-08` è tra questi e confronta variazioni dei contatori, non valori assoluti.
12. `R3.AC12` The suite SHALL includere scenari sul logging: disabilitato, compatto e verbose.
    - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-11` e `R-12` sono tra questi.
13. `R3.AC13` The suite SHALL includere scenari sul ciclo di vita del processo: avvio senza configurazione dei plugin e shutdown ordinato con richieste in corso.
    - Acceptance check: ogni caso elencato ha almeno uno scenario; `R-13` è tra questi.
14. `R3.AC14` The suite SHALL identificare ogni scenario con un nome stabile che riporta, quando esiste, lo scenario di riproduzione o il finding di origine.
    - Acceptance check: l'elenco degli scenari prodotto dalla suite contiene gli ID da `R-01` a `R-14` tranne `R-07` (`C6`) e le sonde P1-P12 citate in `R3.AC4` e `R3.AC5`.

### R4 Bug noti

**User Story:** Come maintainer, voglio che i comportamenti ancora rotti restino nel catalogo senza bloccare la CI, così che ogni correzione li renda verdi e nessun bug venga dimenticato.

#### Acceptance Criteria

1. `R4.AC1` WHERE uno scenario verifica un comportamento rotto da un finding aperto, the suite SHALL riportarlo come bug noto, con l'ID del finding, senza fallire.
   - Acceptance check: con gli scenari di F-01 marcati come bug noti, la suite passa e il riepilogo li elenca sotto F-01.
2. `R4.AC2` IF uno scenario marcato come bug noto passa, THEN the suite SHALL fallire chiedendo di rimuovere il marcatore.
   - Acceptance check: con una correzione simulata che fa passare uno scenario marcato, la suite fallisce e indica lo scenario e il finding.
3. `R4.AC3` IF uno scenario marcato come bug noto fallisce per un errore dell'harness (proxy non avviato, backend non raggiungibile, tempo scaduto), THEN the suite SHALL fallire.
   - Acceptance check: con un proxy che non parte, uno scenario marcato come bug noto fa fallire la suite invece di essere riportato come bug noto.
4. `R4.AC4` WHEN la suite termina, the suite SHALL riepilogare gli scenari passati, falliti e i bug noti raggruppati per finding.
   - Acceptance check: l'output della suite termina con il riepilogo, con il numero di scenari per ciascun finding aperto.

### R5 Esecuzione e integrazione con la CI

**User Story:** Come maintainer, voglio eseguire la suite in locale e in CI con un solo comando, così che ogni pull request venga verificata sugli scenari del proxy.

#### Acceptance Criteria

1. `R5.AC1` WHEN si esegue `make e2e`, the system SHALL eseguire l'intera suite e riportarne l'esito e il riepilogo.
   - Acceptance check: `make e2e` termina con successo e stampa il riepilogo di `R4.AC4`.
2. `R5.AC2` WHEN si passa un filtro su scenario o area, the system SHALL eseguire solo gli scenari corrispondenti.
   - Acceptance check: con il filtro di una sola area l'output contiene solo gli scenari di quell'area.
3. `R5.AC3` The system SHALL eseguire la suite nei controlli esistenti della CI (test con race detector, test ermetici, copertura) a ogni pull request verso `main` e a ogni push su `main`.
   - Acceptance check: i log dei job `test`, `hermetic` e `coverage` riportano gli scenari della suite.
4. `R5.AC4` The system SHALL eseguire la suite con il race detector, in ordine casuale e con il traffico di rete in uscita limitato al loopback.
   - Acceptance check: la suite passa in queste tre condizioni.
5. `R5.AC5` IF una modifica al proxy rompe un comportamento coperto da uno scenario verde, THEN the system SHALL far fallire la CI.
   - Acceptance check: `make ci-selftest` contiene un caso che altera un comportamento coperto (per esempio l'inoltro di un header) e verifica che il controllo che esegue la suite fallisca.

### R6 Riuso

**User Story:** Come autore di una spec, voglio aggiungere scenari senza modificare l'harness, così che ogni correzione arrivi con i propri scenari end-to-end.

#### Acceptance Criteria

1. `R6.AC1` The harness SHALL permettere di aggiungere uno scenario dichiarando configurazione, backend, richieste e verifiche, senza modificare il codice dell'harness.
   - Acceptance check: uno scenario nuovo si aggiunge in un solo file di scenari, senza cambiare i file dell'harness.
2. `R6.AC2` The README SHALL descrivere come scrivere ed eseguire uno scenario e come marcare o togliere un bug noto.
   - Acceptance check: il README contiene una sezione sugli scenari end-to-end con un esempio completo.

## Non-Functional Requirements

- `NFR1` Ermeticità: la suite usa solo connessioni loopback. Verificato da `R1.AC2` e `R5.AC4`.
- `NFR2` Durata: l'intera suite termina in al più 2 minuti in locale con la cache calda; la pipeline di S-02 resta entro 15 minuti. Verificato dalla durata di `make e2e` e delle esecuzioni CI (`R5.AC1`, `R5.AC3`).
- `NFR3` Stabilità: 10 esecuzioni consecutive della suite danno lo stesso esito. Verificato con un'esecuzione ripetuta della suite (`R5.AC4`).
- `NFR4` Diagnosticabilità: un fallimento si capisce dall'output, senza rieseguire lo scenario. Verificato da `R2.AC8`.

## Constraints And Dependencies

- `C1` Nessuna nuova dipendenza: libreria standard, `gorilla/websocket` e `testify`, già presenti (constitution).
- `C2` L'unica modifica al codice di produzione ammessa è estrarre da `cmd/main.go` la costruzione della catena e del server, in modo che binario e harness la condividano, senza cambiarne il comportamento.
- `C3` Dito usa stato globale (configurazione corrente, registry delle metriche, canale dei log): gli scenari in-process che ne dipendono non possono girare in parallelo nello stesso processo.
- `C4` Gli scenari con plugin firmati richiedono CGO e la stessa toolchain e le stesse dipendenze del proxy (constitution).
- `C5` Dipendenze: S-01 (suite ermetica e senza race) e S-02 (controlli della CI, soglie di copertura, lint sul codice nuovo, `make ci-selftest`). Abilita le proof delle spec S-03…S-14 e l'indicatore K-7 del PRD.
- `C6` Il repository è pubblico: gli scenari che dimostrano finding di sicurezza ancora aperti (sezione B del PRD, F-13…F-22) non entrano nel catalogo e li aggiungono le spec che li correggono. Restano quindi fuori `R-07` (F-13), i backend `wss` con CA personalizzata (F-14) e i middleware non critici mancanti (F-15).

## Out Of Scope

- La correzione dei bug: ogni spec rende verdi i propri scenari e toglie i marcatori.
- Gli scenari dei finding di sicurezza ancora aperti (`C6`).
- Test di carico, benchmark e fuzzing.
- Test end-to-end dell'immagine container e dei deployment Kubernetes e OpenShift (S-10).
- Qualsiasi servizio esterno: nessuno scenario esce dal loopback.
