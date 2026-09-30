---
walden_schema_version: v1alpha1
status: approved
approved_at: 2026-09-29T14:47:59Z
last_modified: 2026-09-29T14:50:39Z
approved_fingerprint: sha256:2538295c4e63b73c55bd28a6e6408c506819f5a94d11dd1d4ac2a2674f77fb93
source_design_approved_at: 2026-09-29T14:40:23Z
source_design_fingerprint: sha256:b32171965b631dee0efc7dd2c80fe76868010b5fa06fa8838cfd4a2f0ab1cbf5
---

# Implementation Plan

Le proof usano `env GOTOOLCHAIN=auto` (toolchain da `go.mod`, vincolo `C4`). Le proof ermetiche usano `sandbox-exec`, disponibile solo su macOS (`C5`), con un profilo che permette solo traffico di loopback e socket unix. I task 1-4 seguono il TDD: il test nuovo o modificato si vede fallire prima del fix. I task 5 e 6 sono checkpoint di sola verifica.

- [x] 1. `LimitedBuffer`: `Read` con lock esclusivo e test di consegna unica
  - TDD: scrivere `TestLimitedBuffer_ConcurrentReadsExactlyOnce` (32.768 byte, ogni valore 0-255 ripetuto 128 volte; 8 goroutine leggono a blocchi di 7 byte fino a `io.EOF`; istogramma letto uguale a quello scritto) e vederlo fallire con `-race` sul codice attuale; poi portare `Read` da `RLock` a `Lock`.
  - Requirements: `R1.AC1`, `R1.AC2`, `NFR2`
  - Design: Architecture, Verification Plan
  - Verification:
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-race", "-count=1", "-v", "-run", "^TestLimitedBuffer_ConcurrentAccess$", "./writer"]
      expect_output: "--- PASS: TestLimitedBuffer_ConcurrentAccess"
      covers: ["R1.AC1"]
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-race", "-count=1", "-v", "-run", "^TestLimitedBuffer_ConcurrentReadsExactlyOnce$", "./writer"]
      expect_output: "--- PASS: TestLimitedBuffer_ConcurrentReadsExactlyOnce"
      covers: ["R1.AC1", "R1.AC2"]

- [x] 2. Rimuovere `TestConcurrentWrites`
  - Il test usa `writer.ResponseWriter` da più goroutine, fuori dal contratto (`C1`). Dopo il task 1 è l'unica causa per cui il package `writer` fallisce con `-race`. Il conteggio dei byte su più scritture resta coperto da `TestStreamingMode`.
  - Requirements: `R2.AC1`
  - Design: Architecture, Options Considered
  - Verification:
    - command: ["grep", "-n", "func TestConcurrentWrites", "writer/writer_test.go"]
      expect_exit: 1
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-race", "-count=1", "-v", "./writer"]
      expect_output: "--- PASS: TestStreamingMode"
      covers: ["R2.AC1"]

- [x] 3. Sincronizzare `TestWatchConfig`
  - TDD: con `-race` il test fallisce oggi. La callback invia la nuova config su un canale bufferizzato con invio non bloccante; il test attende sul canale con un timeout di 10 s e verifica `port` = `"9090"`. L'attesa iniziale di 3 s resta (S-06).
  - Requirements: `R2.AC1`
  - Design: Architecture, Failure Modes And Tradeoffs
  - Verification:
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-race", "-count=1", "-v", "-run", "^TestWatchConfig$", "./config"]
      expect_output: "--- PASS: TestWatchConfig"
      covers: ["R2.AC1"]

- [x] 4. `TestDynamicProxyHandler` con backend su loopback
  - TDD: nella sandbox di rete il test fallisce oggi. Il target diventa un `httptest.NewServer` che registra le richieste; il test verifica status 200, una sola richiesta ricevuta, metodo `GET` e path `/`. `setupTestConfig` riceve l'URL del backend come parametro. Il primo passo della proof compila i test fuori dalla sandbox per popolare le cache.
  - Requirements: `R3.AC2`, `NFR1`
  - Design: Architecture, Verification Plan
  - Verification:
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-run", "^$", "./handlers"]
    - command: ["sandbox-exec", "-p", "(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip \"localhost:*\"))(allow network-outbound (remote unix-socket))", "env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-v", "-run", "^TestDynamicProxyHandler$", "./handlers"]
      expect_output: "--- PASS: TestDynamicProxyHandler"
      covers: ["R3.AC2"]

- [x] 5. Suite completa con race detector
  - Checkpoint senza modifiche al codice; dipende dai task 1-4. Con `-race` qualunque segnalazione fa fallire il package; il marker dimostra che il test nuovo del task 1 fa parte dell'esecuzione. Seguono `go vet` e il controllo che `go.mod` e `go.sum` siano già in ordine (`C3`).
  - Requirements: `R2.AC1`, `NFR2`
  - Design: Verification Plan
  - Verification:
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-race", "-count=1", "-v", "./..."]
      expect_output: "--- PASS: TestLimitedBuffer_ConcurrentReadsExactlyOnce"
      covers: ["R2.AC1"]
    - command: ["env", "GOTOOLCHAIN=auto", "go", "vet", "./..."]
    - command: ["env", "GOTOOLCHAIN=auto", "go", "mod", "tidy", "-diff"]

- [x] 6. Suite completa senza rete esterna
  - Checkpoint senza modifiche al codice; dipende dal task 4. Il primo passo compila tutti i test fuori dalla sandbox. Il controllo negativo dimostra che la rete era davvero bloccata: nella sandbox la connessione a `192.0.2.1` (TEST-NET-1) fallisce subito con exit 7; senza blocco andrebbe in timeout con exit 28.
  - Requirements: `R3.AC1`, `NFR1`
  - Design: Architecture, Verification Plan
  - Verification:
    - command: ["env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-run", "^$", "./..."]
    - command: ["sandbox-exec", "-p", "(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip \"localhost:*\"))(allow network-outbound (remote unix-socket))", "env", "GOTOOLCHAIN=auto", "go", "test", "-count=1", "-v", "./..."]
      expect_output: "--- PASS: TestDynamicProxyHandler"
      covers: ["R3.AC1"]
    - command: ["sandbox-exec", "-p", "(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip \"localhost:*\"))(allow network-outbound (remote unix-socket))", "/usr/bin/curl", "-sS", "-m", "3", "-o", "/dev/null", "http://192.0.2.1/"]
      expect_exit: 7
      expect_output: "Failed to connect to 192.0.2.1"
      covers: ["R3.AC1"]
