$ErrorActionPreference='Stop'
& ./go.ps1 run scripts/icon.go
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$rsrc=Join-Path $PSScriptRoot '.tools/gopath/bin/rsrc.exe'
if (-not (Test-Path -LiteralPath $rsrc)) {
 & ./go.ps1 install github.com/akavel/rsrc@v0.10.2
 if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
& $rsrc -ico assets/icon.ico -arch amd64 -o icon_windows_amd64.syso
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
