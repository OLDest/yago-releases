$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
if ([System.IO.File]::Exists((Join-Path $root '.tools/go/bin/go.exe'))) { & (Join-Path $root 'prepare-toolchain.ps1'); return }
$zip = Join-Path $root 'go-toolchain.zip'
$expected = '178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e'
$stream = [System.IO.File]::OpenRead($zip)
try { $hash = [System.BitConverter]::ToString([System.Security.Cryptography.SHA256]::Create().ComputeHash($stream)).Replace('-', '').ToLower() } finally { $stream.Dispose() }
if ($hash -ne $expected) { throw 'Go archive checksum mismatch' }
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Directory]::CreateDirectory((Join-Path $root '.tools')) | Out-Null
[System.IO.Compression.ZipFile]::ExtractToDirectory($zip, (Join-Path $root '.tools'))

& (Join-Path $root 'prepare-toolchain.ps1')
