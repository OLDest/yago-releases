$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & ./build-icons.ps1
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & ./go.ps1 build -mod=readonly -buildvcs=false -trimpath -ldflags '-s -w -H=windowsgui' -o YaGo.exe .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally { Pop-Location }
