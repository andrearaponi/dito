---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-29T14:34:19Z
last_modified: 2026-09-29T14:34:19Z
approved_fingerprint: sha256:82c14fae0d2e8752f8b920123af75cdd536a4fdc83274aa1bbdff8e13ec6d7f6
---

# Requirements Document

## Introduction

La suite di test di Dito passa senza race detector, ma con il race detector attivo falliscono 3 test, e un test dipende da Internet (`http://example.com`). Finché è così, `go test -race` non può fare da gate: né per la CI (S-02), né per le proof delle spec successive.

Questa spec ripristina tre proprietà:

- l'accesso concorrente sicuro a `LimitedBuffer`, che la sua documentazione dichiara *"thread-safe"* (F-27);
- una suite priva di data race (F-41: `TestWatchConfig`, `TestConcurrentWrites`);
- una suite ermetica, che non contatta la rete esterna (F-42: `TestDynamicProxyHandler`).

Fonte: SEPTEMBER-STATE F-27, F-41, F-42 (spec candidata S-01).

<!-- assumed: spec creata su richiesta esplicita dell'utente anche se si tratta di manutenzione che ripristina il contratto (source: conversazione del 2026-09-29, SEPTEMBER-STATE §8) -->

## Requirements

### R1 Accesso concorrente sicuro a `LimitedBuffer`

**User Story:** Come utilizzatore del package `writer`, voglio che `LimitedBuffer` sia davvero sicuro in accesso concorrente, così che letture e scritture parallele non corrompano i dati né causino panic.

#### Acceptance Criteria

1. `R1.AC1` WHEN più goroutine chiamano in parallelo metodi di lettura e di scrittura sullo stesso `LimitedBuffer`, the system SHALL completare ogni chiamata senza data race.
   - Acceptance check: con il race detector attivo, un'esecuzione parallela di `Write`, `Read`, `String`, `Bytes`, `Len` e `Available` sullo stesso buffer termina senza segnalazioni e senza panic.
2. `R1.AC2` WHEN più goroutine eseguono `Read` in parallelo sullo stesso `LimitedBuffer`, the system SHALL restituire ogni byte presente nel buffer a un solo chiamante.
   - Acceptance check: dopo letture parallele fino a svuotare il buffer, i byte restituiti nel complesso coincidono con quelli scritti, senza duplicati né mancanze.

### R2 Suite priva di data race

**User Story:** Come maintainer, voglio che la suite passi con il race detector attivo, così da poterla usare come gate in CI e nelle proof delle spec Walden.

#### Acceptance Criteria

1. `R2.AC1` The test suite SHALL superare l'esecuzione completa con il race detector attivo senza segnalazioni di data race.
   - Acceptance check: l'esecuzione di tutti i package con il race detector termina con successo e il suo output non contiene segnalazioni `DATA RACE`.

### R3 Suite ermetica

**User Story:** Come maintainer, voglio che la suite non dipenda da servizi esterni, così che dia lo stesso risultato in locale, in CI e offline.

#### Acceptance Criteria

1. `R3.AC1` WHILE le connessioni di rete in uscita sono limitate al loopback, the test suite SHALL superare l'esecuzione completa.
   - Acceptance check: con il traffico verso indirizzi non di loopback bloccato, tutti i package superano i test (oggi `handlers` fallisce in queste condizioni).
2. `R3.AC2` WHEN il test del proxy handler inoltra una richiesta, the test suite SHALL consegnarla a un backend su loopback controllato dal test.
   - Acceptance check: il test verifica che il proprio backend locale abbia ricevuto la richiesta con il metodo e il path attesi, e fallisce se la richiesta non lo raggiunge.

## Non-Functional Requirements

- `NFR1` Affidabilità: il risultato della suite non dipende dalla disponibilità di servizi di rete esterni (verificato da `R3.AC1` e `R3.AC2`).
- `NFR2` Rilevamento delle race: l'esecuzione con il race detector può fare da gate bloccante, perché un esito rosso indica un problema reale e non un difetto dei test (verificato da `R1.AC1`, `R1.AC2` e `R2.AC1`).

## Constraints And Dependencies

- `C1` `writer.ResponseWriter` segue il contratto di `http.ResponseWriter`: non deve supportare chiamate concorrenti, e i test non devono usarlo da più goroutine in parallelo. Riguarda `TestConcurrentWrites` (F-41). <!-- assumed: contratto di http.ResponseWriter (source: SEPTEMBER-STATE F-41) -->
- `C2` Nessuna modifica all'API esportata dei package di produzione.
- `C3` Nessuna nuova dipendenza (constitution).
- `C4` Le verifiche richiedono Go ≥ 1.27.1. Con `GOTOOLCHAIN=local` la toolchain va installata; in alternativa si usa `GOTOOLCHAIN=go1.27.1`.
- `C5` La verifica di `R3.AC1` richiede un meccanismo che blocchi il traffico non di loopback durante i test. Su macOS è disponibile `sandbox-exec`; per la CI Linux l'equivalente si definisce nel design o in S-02.

## Out Of Scope

- Rendere `WatchConfig` cancellabile e il suo test deterministico, per esempio con `testing/synctest`: dipende dalla cancellazione del watcher (S-06, F-26). `TestWatchConfig` resta basato su attese reali.
- La goroutine di `WatchConfig` che resta attiva dopo il test (S-06).
- La coerenza di `Len` e `Available` dopo `Read` in `LimitedBuffer` (F-50, S-03).
- Rendere `writer.ResponseWriter` sicuro per chiamate concorrenti (vedi `C1`).
- Aumento della copertura, lint dei test e gate `-race` in CI (S-02).
