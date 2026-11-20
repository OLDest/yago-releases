# Go 1.27.4: lazy entropy scratch buffer

Release builds use the stock Go 1.27.4 compiler with one source patch applied by prepare-toolchain.ps1. Only the project-local toolchain is changed. SHA-256 checks reject unknown input or modified patch files.

The FIPS entropy implementation declares a static 32 MiB scratch buffer. On this Windows PC its pages became resident after creating even a blank Gio window. The exact component that touched those pages was not identified.

The patch replaces the static array with sync.OnceValue returning a pointer to the same-sized array. It is allocated on first getEntropy call and remains shared across calls. The entropy algorithm, operating-system randomness, TLS verification and Ed25519 verification are unchanged. No FIPS mode is forced. This patched build is not claimed to be a FIPS-certified cryptographic module.

Source: Go src/crypto/internal/fips140/drbg/entropy_fips140.go. The complete patched file and original BSD license are included here. Compare the file with stock Go 1.27.4 to review the change. Go update requires a fresh review and checksum update.

Validation: native window residency, standard-library crypto/rand, crypto/ed25519, crypto/tls; FIPS-mode crypto/rand and crypto/ed25519 tests; YaGo playback and signed updater tests. See MEMORY_INVESTIGATION.md for measured results.
