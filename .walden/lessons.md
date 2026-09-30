# Walden Lessons

Review this file before non-trivial work when the current request matches past mistakes, rejections, or validation failures.

## Lessons

<!-- Append entries with: walden lesson log --feature <name> --phase <phase> --trigger "..." --lesson "..." --guardrail "..." -->
### 2026-09-29T14:55:52Z | test-suite-hygiene | execute
- Trigger: SEPTEMBER-STATE.md (tracking S-01) aggiornato dopo il walden verify di chiusura: dopo il commit release check ha segnalato tutti e 6 i task come stale-code. Ricostruendo il manifest si ottiene l'identità registrata ripristinando il PRD pre-verify: il commit in sé non ha cambiato l'identità.
- Lesson: L'identità del codice di Walden è un manifest di tutto il working tree (file tracciati e non tracciati non ignorati, symlink compresi), esclusa solo .walden/: anche una modifica a documentazione o PRD invalida le evidenze.
- Guardrail: Completare ogni modifica ai deliverable (PRD, README, docs) prima del walden verify finale; dopo la verify solo commit. Se cambia qualcosa fuori da .walden/, rieseguire walden verify prima di committare.

### 2026-09-30T07:29:50Z | ci-quality-gates | execute
- Trigger: Per ispezionare le variabili esportate ho eseguito 'make -f - <<EOF include Makefile ... EOF' senza un target esplicito: e' partito il target di default 'setup', che ha generato chiavi in bin/, compilato e firmato il plugin dentro plugins/hello-plugin/ e riscritto bin/config.yaml(.bak). File rimossi; il .bak rigenerato e' identico all'originale (deterministico).
- Lesson: Il primo target del Makefile di Dito e' 'setup', che ha effetti collaterali (chiavi, binari, plugin nel repository, config in bin/): invocare make senza goal non e' mai un'operazione di sola lettura.
- Guardrail: Invocare sempre make con un target esplicito; per ispezionare usare 'make -n <target>' oppure 'make -f - <target>'. Dopo un errore del genere, confrontare bin/ e plugins/ con lo stato precedente prima di proseguire.

### 2026-09-30T17:34:09Z | ci-quality-gates | execute
- Trigger: Task 7.3 proof (observe-github.sh runs) failed on the first real run: the push log did contain 'tools-check: ok (go1.27.1)', but 'gh run view --log | grep -q' under set -o pipefail exited 141 because gh got SIGPIPE when grep stopped at the first match.
- Lesson: Under pipefail, 'producer | grep -q' is a false negative whenever the producer writes more than a pipe buffer after the match; scripts that can only run after a push were never exercised on real data before their proof.
- Guardrail: In pipefail scripts match against a captured variable or file (case, grep FILE), never 'large-producer | grep -q'; exercise post-push observation scripts against a real or recorded run log before relying on their proof.

### 2026-09-30T18:21:20Z | ci-quality-gates | release
- Trigger: After committing the SEPTEMBER-STATE.md removal, S-01 and S-02 evidence turned stale-code although no file content changed: walden verify had run while the removal was only staged (git rm --cached, file still on disk and ignored).
- Lesson: Walden's code identity treats a file that is still in HEAD as tracked even if it was removed from the index and is now ignored: verifying before committing a removal records an identity that the commit invalidates (reproduced: staged-only removal gives the pre-removal identity, the committed removal a new one).
- Guardrail: Commit a removal of tracked files (or any change of what git tracks) before the final walden verify; then verify on the committed state and commit only the evidence.

