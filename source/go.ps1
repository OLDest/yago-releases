$ErrorActionPreference = 'Stop'
$env:GOROOT = Join-Path $PSScriptRoot '.tools/go'
& (Join-Path $PSScriptRoot 'prepare-toolchain.ps1') -ToolchainRoot $env:GOROOT
$env:GOPATH = Join-Path $PSScriptRoot '.tools/gopath'
$env:GOCACHE = Join-Path $PSScriptRoot '.tools/build-cache'
$env:CGO_ENABLED = '0'
& (Join-Path $env:GOROOT 'bin/go.exe') @args
exit $LASTEXITCODE
