# Walden Lessons

Review this file before non-trivial work when the current request matches past mistakes, rejections, or validation failures.

## Lessons

<!-- Append entries with: walden lesson log --feature <name> --phase <phase> --trigger "..." --lesson "..." --guardrail "..." -->
### 2026-09-29T14:55:52Z | test-suite-hygiene | execute
- Trigger: SEPTEMBER-STATE.md (tracking S-01) aggiornato dopo il walden verify di chiusura: dopo il commit release check ha segnalato tutti e 6 i task come stale-code. Ricostruendo il manifest si ottiene l'identità registrata ripristinando il PRD pre-verify: il commit in sé non ha cambiato l'identità.
- Lesson: L'identità del codice di Walden è un manifest di tutto il working tree (file tracciati e non tracciati non ignorati, symlink compresi), esclusa solo .walden/: anche una modifica a documentazione o PRD invalida le evidenze.
- Guardrail: Completare ogni modifica ai deliverable (PRD, README, docs) prima del walden verify finale; dopo la verify solo commit. Se cambia qualcosa fuori da .walden/, rieseguire walden verify prima di committare.

