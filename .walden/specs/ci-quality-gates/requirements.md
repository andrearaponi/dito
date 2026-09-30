---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-30T06:43:35Z
last_modified: 2026-09-30T06:43:35Z
approved_fingerprint: sha256:e224fe831b3ea48915297cede6e8c2eb0aa160130318bb30ea99dc4132d14390
---

# Requirements Document

## Introduction

Dito non ha una CI (F-40): nessuno dei difetti raccolti in SEPTEMBER-STATE poteva essere intercettato prima del merge. Dopo S-01 la suite di test passa con il race detector ed è ermetica, quindi può fare da gate.

Questa spec introduce:

- una **pipeline** di controlli bloccanti su ogni pull request e su `main`: build, vet, test con race detector senza rete esterna, ordine di `go.mod`, lint, vulnerabilità, immagine container, smoke end-to-end dei plugin, copertura;
- gli stessi controlli eseguibili in locale;
- l'aggiornamento automatico di dipendenze, GitHub Action e immagini base.

Fonte: SEPTEMBER-STATE F-40, F-43, F-44, F-45 (spec candidata S-02); vincolo `C5` di S-01 (verifica ermetica su Linux).

<!-- assumed: la pipeline gira su GitHub Actions (source: remote github.com/andrearaponi/dito; workflow validate-walden.yml già presente) -->

## Requirements

### R1 Pipeline su pull request e su `main`

**User Story:** Come maintainer, voglio che ogni pull request e ogni commit su `main` vengano verificati automaticamente, così che un difetto venga intercettato prima del merge.

#### Acceptance Criteria

1. `R1.AC1` WHEN viene aperta o aggiornata una pull request verso `main`, the system SHALL eseguire tutti i controlli bloccanti sull'ultimo commit della pull request.
   - Acceptance check: l'ultimo commit di ogni pull request riporta l'esito di ciascun controllo bloccante.
2. `R1.AC2` WHEN un commit arriva su `main`, the system SHALL eseguire tutti i controlli bloccanti su quel commit.
   - Acceptance check: ogni commit su `main` ha un'esecuzione della pipeline con l'esito di ciascun controllo.
3. `R1.AC3` The system SHALL eseguire i controlli con la toolchain Go dichiarata in `go.mod`.
   - Acceptance check: il log di ogni esecuzione riporta la stessa versione di Go dichiarata in `go.mod`.
4. `R1.AC4` The system SHALL eseguire i job di controllo con un token dai permessi di sola lettura.
   - Acceptance check: i workflow dichiarano permessi di sola lettura e nessun job di controllo ottiene permessi di scrittura.
5. `R1.AC5` The system SHALL riferire ogni GitHub Action di terze parti tramite un commit SHA immutabile.
   - Acceptance check: nessun riferimento `uses:` nei workflow del repository punta a un tag o a un branch.

### R2 Build, test e integrità dei moduli

**User Story:** Come maintainer, voglio che la pipeline fallisca a ogni errore di compilazione, di analisi o di test, così che `main` resti sempre compilabile e verde.

#### Acceptance Criteria

1. `R2.AC1` IF la compilazione del proxy, del `plugin-signer` o del plugin di esempio fallisce, THEN the system SHALL far fallire il controllo di build.
   - Acceptance check: con un errore di compilazione introdotto di proposito in uno dei tre, il controllo di build fallisce.
2. `R2.AC2` IF `go vet` segnala un problema, THEN the system SHALL far fallire il controllo di analisi.
   - Acceptance check: con un problema rilevabile da `go vet` introdotto di proposito, il controllo fallisce.
3. `R2.AC3` IF un test fallisce o il race detector segnala una data race, THEN the system SHALL far fallire il controllo dei test.
   - Acceptance check: con un test rosso o una data race introdotti di proposito, il controllo fallisce.
4. `R2.AC4` The system SHALL eseguire la suite di test con il traffico di rete in uscita limitato al loopback.
   - Acceptance check: la suite passa in queste condizioni e, nello stesso controllo, una connessione di prova verso un indirizzo esterno fallisce.
5. `R2.AC5` IF `go.mod` o `go.sum` del modulo principale o del plugin di esempio differiscono dall'esito di `go mod tidy`, THEN the system SHALL far fallire il controllo dei moduli.
   - Acceptance check: con una dipendenza non usata aggiunta di proposito a uno dei due `go.mod`, il controllo fallisce.
6. `R2.AC6` IF la costruzione dell'immagine container fallisce, THEN the system SHALL far fallire il controllo dell'immagine.
   - Acceptance check: con un errore introdotto di proposito nel `Dockerfile`, il controllo fallisce.

### R3 Lint e vulnerabilità

**User Story:** Come maintainer, voglio che nessuna modifica aggiunga nuovi problemi di lint o vulnerabilità note, senza essere bloccato dal debito esistente.

#### Acceptance Criteria

1. `R3.AC1` IF una modifica introduce una nuova segnalazione di golangci-lint, THEN the system SHALL far fallire il controllo di lint.
   - Acceptance check: con una violazione nuova introdotta di proposito, il controllo di lint fallisce e indica file e linter.
2. `R3.AC2` IF le uniche segnalazioni di golangci-lint riguardano codice già presente prima della modifica, THEN the system SHALL considerare superato il controllo di lint.
   - Acceptance check: su una modifica che non tocca il codice con le 105 segnalazioni preesistenti, il controllo di lint passa.
3. `R3.AC3` IF govulncheck rileva una vulnerabilità raggiungibile dal codice, THEN the system SHALL far fallire il controllo delle vulnerabilità.
   - Acceptance check: con una dipendenza fissata di proposito a una versione vulnerabile e raggiungibile, il controllo fallisce.
4. `R3.AC4` WHEN scatta l'esecuzione programmata settimanale, the system SHALL eseguire il controllo delle vulnerabilità sull'ultimo commit di `main`.
   - Acceptance check: lo storico della pipeline mostra un'esecuzione programmata per ogni settimana, anche senza nuovi commit.

### R4 Plugin verificati end-to-end

**User Story:** Come sviluppatore di plugin, voglio che ogni modifica verifichi che un plugin firmato si carichi e funzioni nel proxy, così che incompatibilità di toolchain, dipendenze o firma emergano subito.

#### Acceptance Criteria

1. `R4.AC1` WHEN la pipeline viene eseguita, the system SHALL caricare nel proxy il plugin di esempio compilato e firmato nella stessa esecuzione.
   - Acceptance check: se il proxy non carica il plugin (firma non valida, toolchain o dipendenze diverse tra host e plugin), il controllo fallisce.
2. `R4.AC2` WHEN il proxy avviato dalla pipeline riceve una richiesta per una location che usa il middleware del plugin, the system SHALL restituire la risposta con l'effetto del middleware.
   - Acceptance check: la risposta contiene l'header impostato dal plugin di esempio; se manca, il controllo fallisce.
3. `R4.AC3` The system SHALL firmare il plugin di prova con una coppia di chiavi generata per la singola esecuzione.
   - Acceptance check: il controllo non usa segreti del repository e la chiave privata non sopravvive all'esecuzione.

### R5 Copertura

**User Story:** Come maintainer, voglio vedere la copertura a ogni esecuzione e impedire che peggiori, così che i test crescano insieme al codice.

#### Acceptance Criteria

1. `R5.AC1` WHEN la pipeline esegue i test, the system SHALL pubblicare la copertura di ogni package e quella totale.
   - Acceptance check: ogni esecuzione espone una tabella con la copertura per package e il totale.

2. `R5.AC2` IF la copertura di un package scende sotto la soglia registrata per quel package, THEN the system SHALL far fallire il controllo della copertura.
   - Acceptance check: con un test rimosso di proposito in modo che un package scenda sotto la sua soglia, il controllo fallisce indicando package, soglia e valore misurato.
3. `R5.AC3` IF una modifica abbassa la soglia registrata di un package ancora presente, THEN the system SHALL far fallire il controllo della copertura.
   - Acceptance check: con una soglia abbassata di proposito rispetto a `main`, il controllo fallisce; eliminare un package elimina anche la sua soglia senza errori.

<!-- assumed: gate di copertura secondo l'opzione (a) scelta dall'utente il 2026-09-29: una soglia minima per package, inizialmente pari alla copertura misurata oggi (totale 50,4%), che può solo salire; le spec che aggiungono test alzano le soglie. Le soglie di K-5 restano un obiettivo, non un gate (source: conversazione del 2026-09-29) -->

### R6 Stessi controlli in locale

**User Story:** Come sviluppatore, voglio eseguire in locale gli stessi controlli della pipeline con le stesse versioni degli strumenti, così da scoprire i problemi prima di aprire una pull request.

#### Acceptance Criteria

1. `R6.AC1` The system SHALL rendere eseguibile in locale, con un target `make`, ogni controllo bloccante della pipeline.
   - Acceptance check: per ogni controllo della pipeline esiste un target `make` che, eseguito in locale, controlla la stessa cosa con lo stesso esito.
2. `R6.AC2` The system SHALL usare in CI e in locale le stesse versioni degli strumenti di verifica, dichiarate in un solo punto del repository.
   - Acceptance check: cambiando la versione di uno strumento in quel punto, cambia sia in CI sia in locale; nessuna versione è ripetuta altrove.
3. `R6.AC3` WHEN uno sviluppatore esegue il target aggregato dei controlli, the system SHALL eseguire in sequenza tutti i controlli bloccanti che non richiedono un motore container.
   - Acceptance check: il target aggregato esegue build, vet, moduli, test, lint, vulnerabilità, smoke dei plugin e copertura, e fallisce al primo controllo rosso.

### R7 Aggiornamento automatico delle dipendenze

**User Story:** Come maintainer, voglio ricevere pull request di aggiornamento per dipendenze Go, GitHub Action e immagini base, così da restare aggiornato sulle correzioni di sicurezza senza controlli manuali.

#### Acceptance Criteria

1. `R7.AC1` WHEN viene pubblicata una nuova versione di una dipendenza Go del modulo principale, the system SHALL proporre una pull request di aggiornamento.
   - Acceptance check: entro il ciclo di controllo configurato compare una pull request che aggiorna la dipendenza.
2. `R7.AC2` WHEN viene pubblicata una nuova versione di una GitHub Action usata dai workflow, the system SHALL proporre una pull request di aggiornamento.
   - Acceptance check: la pull request aggiorna il commit SHA e la versione indicata accanto.
3. `R7.AC3` WHEN viene pubblicata una nuova versione di un'immagine base del `Dockerfile`, the system SHALL proporre una pull request di aggiornamento.
   - Acceptance check: la pull request aggiorna il riferimento all'immagine nel `Dockerfile`.
4. `R7.AC4` WHEN una pull request di aggiornamento modifica le dipendenze del modulo principale, the system SHALL allineare nella stessa pull request il modulo del plugin di esempio.
   - Acceptance check: la pull request di aggiornamento supera lo smoke dei plugin (`R4`) senza interventi manuali.
5. `R7.AC5` WHEN sono disponibili più aggiornamenti minor o patch per lo stesso ecosistema, the system SHALL proporli in un'unica pull request.
   - Acceptance check: aggiornamenti minor e patch simultanei dello stesso ecosistema arrivano in una sola pull request.

## Non-Functional Requirements

- `NFR1` Durata: una normale esecuzione della pipeline termina entro 15 minuti (verificato sulle esecuzioni di `R1.AC1` e `R1.AC2`).
- `NFR2` Sicurezza della pipeline: permessi minimi, azioni immutabili e nessun segreto richiesto per i controlli delle pull request (verificato da `R1.AC4`, `R1.AC5` e `R4.AC3`).
- `NFR3` Affidabilità del segnale: un esito rosso indica un problema reale e non una dipendenza da rete o da versioni non fissate (verificato da `R2.AC4`, `R2.AC3` e `R6.AC2`).
- `NFR4` Riproducibilità: gli stessi controlli, con le stesse versioni, in CI e in locale (verificato da `R1.AC3`, `R6.AC1` e `R6.AC2`).

## Constraints And Dependencies

- `C1` La pipeline gira su GitHub Actions, dove esiste già il workflow `validate-walden.yml` generato da Walden: va mantenuto. <!-- assumed: source remote e .github/ -->
- `C2` Gli strumenti di verifica (golangci-lint, govulncheck e simili) non entrano nel grafo delle dipendenze del `go.mod` principale, che determina anche le versioni condivise con i plugin; nessuna nuova dipendenza senza approvazione (constitution). Questo precisa il tema "direttive `tool` in `go.mod`" del PRD: le versioni restano fissate (`R6.AC2`) ma fuori dal modulo principale. <!-- assumed: source constitution, Hard Rules -->
- `C3` Runner Linux x86-64 con CGO disponibile, richiesto dal race detector e dai plugin; l'immagine si costruisce solo per `linux/amd64`, come nel `Dockerfile`.
- `C4` golangci-lint deve essere compilato con Go ≥ 1.27 per analizzare il modulo.
- `C5` Il segnale di lint "solo segnalazioni nuove" dipende dal confronto con il ramo di destinazione (`main`). <!-- assumed: baseline che blocca solo le issue nuove (source: SEPTEMBER-STATE S-02) -->
- `C6` Rendere obbligatori i controlli per il merge (protezione del branch `main`) è un'impostazione del repository che deve attivare l'owner: la spec la documenta ma non può applicarla.
- `C7` Le proof locali di Walden non possono eseguire GitHub Actions: in locale si verificano la configurazione dei workflow e i target `make`; trigger, durata e bot di aggiornamento si osservano su GitHub dopo il push, che resta a cura dell'owner.

## Out Of Scope

- Pubblicazione di release, push e firma dell'immagine, versione iniettata nel binario, `-trimpath` (S-10).
- Correzione delle 105 segnalazioni di lint esistenti (M-01 e spec di area) e aumento della copertura (spec di area).
- Merge automatico delle pull request di aggiornamento.
- SAST aggiuntivi (CodeQL, Trivy) e review AI in CI.
- Test su più versioni di Go: host e plugin richiedono la stessa toolchain di `go.mod`. <!-- assumed: una sola versione di Go in CI -->
