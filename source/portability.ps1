$ErrorActionPreference = 'Stop'
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
try {
    $env:GOARCH = 'amd64'
    foreach ($targetOS in @('linux', 'darwin')) {
        $env:GOOS = $targetOS
        foreach ($package in @('music', 'audio', 'store')) {
            & (Join-Path $PSScriptRoot 'go.ps1') test -c -o (Join-Path $PSScriptRoot ".tools/check-$targetOS-$package") "./internal/$package"
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
    }
} finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
}
