# Public release repository policy

This repository must expose only the latest published YaGo version.
- Publish source/ from the matching release source archive, without private Git history.
- Replace main with a parentless snapshot commit; never append development commits.
- Retain exactly one release and one tag, both for the latest version.
- Verify the latest binaries, source archive and signed update manifest before removing older releases and tags.
- After publication, verify one branch, one parentless commit, one tag and one release; verify every source blob against the release archive.
- Never modify the private development repository or erase its history for this policy.
