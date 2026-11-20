$ErrorActionPreference = 'Stop'
$env:YAGO_TEST_MP3 = Join-Path $PSScriptRoot '.tools/gopath/pkg/mod/github.com/hajimehoshi/go-mp3@v0.3.4/example/classic.mp3'
& (Join-Path $PSScriptRoot 'go.ps1') test -count=1 ./...
exit $LASTEXITCODE
