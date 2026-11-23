$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    & ./go.ps1 build -buildvcs=false -trimpath -ldflags '-s -w -X main.windowMode=phone' -o .tools/YaGo-phone-preview.exe .
    exit $LASTEXITCODE
} finally { Pop-Location }
