param([string]$ToolchainRoot = (Join-Path $PSScriptRoot '.tools/go'))
$ErrorActionPreference='Stop'
$sourcePath=Join-Path $ToolchainRoot 'src/crypto/internal/fips140/drbg/entropy_fips140.go'
$patchPath=Join-Path $PSScriptRoot 'toolchain-patches/entropy_fips140.go.txt'
function FileHash([string]$path){$stream=[IO.File]::OpenRead($path);try{return [BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($stream)).Replace('-','').ToLowerInvariant()}finally{$stream.Dispose()}}
$hash=FileHash $sourcePath
if($hash -eq '194f382aa98025206df0c737ca2b29610bde6b1cdeda4faa0ccd0f99e4c75ea0'){return}
if($hash -ne 'bd3c834a29e31c93e56d81da54d4a874561814a86550de1456c8d6d40a9c5fda'){throw 'Unexpected Go crypto source. Use the pinned Go 1.27.4 toolchain; review patch before updating Go.'}
if((FileHash $patchPath) -ne '194f382aa98025206df0c737ca2b29610bde6b1cdeda4faa0ccd0f99e4c75ea0'){throw 'Toolchain patch checksum mismatch'}
[IO.File]::Copy($patchPath,$sourcePath,$true)
Write-Output 'Applied lazy crypto entropy scratch allocation (Go 1.27.4).'
