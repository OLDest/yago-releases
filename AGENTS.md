# Public release repository policy

This repository must expose only the latest published YaGo version.
- The workflow `.github/workflows/latest-only.yml` enforces this policy on releases and pushes. Preserve `.github/` when publishing; never replace or remove the retention workflow or its script.
- The hourly reconciliation also handles publications made with GITHUB_TOKEN, which do not trigger another workflow. A publisher using GITHUB_TOKEN should explicitly dispatch `latest-only.yml` after uploading every asset.
- Publish source/ from the matching release source archive, without private Git history.
- Replace main with a parentless snapshot commit; never append development commits.
- Retain exactly one release and one tag, both for the latest version.
- Verify the latest binaries, source archive and signed update manifest before removing older releases and tags.
- After publication, verify one branch, one parentless commit, one tag and one release; verify every source blob against the release archive.
- Never modify the private development repository or erase its history for this policy.
- Generate README from `.github/README.template.md`: describe current capabilities, not a history of releases. Keep the independence and GPL disclaimer first. Do not advertise local MP3/FLAC filenames. Preserve the template along with the workflow when publishing.
- `SHA256SUMS.txt` must cover EXE and APK. Additional assets may be covered by this file or by their verified GitHub SHA-256 digests; the update manifest signature and binary hashes must always be verified.
