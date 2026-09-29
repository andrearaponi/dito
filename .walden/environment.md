# Environment

Probe diagnostiche che Walden registra insieme alle evidenze. Ogni voce di elenco di questo file viene interpretata come probe, quindi il testo descrittivo va scritto in paragrafi.

Le probe riportano la toolchain Go locale, la toolchain effettiva richiesta da go.mod (con GOTOOLCHAIN=auto, come nelle proof), l'impostazione GOTOOLCHAIN della macchina e la disponibilità degli strumenti usati dalle proof ermetiche (sandbox-exec e curl, solo macOS).

- go: ["go", "version"]
- go-effective: ["env", "GOTOOLCHAIN=auto", "go", "version"]
- go-toolchain-setting: ["go", "env", "GOTOOLCHAIN"]
- sandbox-exec: ["sh", "-c", "command -v sandbox-exec"]
- curl: ["/usr/bin/curl", "--version"]
