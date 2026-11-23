$ErrorActionPreference='Stop'
if([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT){throw 'Windows is required'}
$qa=Join-Path $PSScriptRoot '.tools'
[IO.Directory]::CreateDirectory($qa) | Out-Null
$exe=Join-Path $qa 'zvuk-login-gui-smoke.exe'
Push-Location $PSScriptRoot
$previous=$env:YAGO_BROWSER_TEST
try {
    & ./go.ps1 test -c -buildvcs=false -ldflags '-H=windowsgui' -o $exe ./internal/zvuk
    if($LASTEXITCODE -ne 0){throw 'GUI smoke executable compilation failed'}
    $env:YAGO_BROWSER_TEST='1'
    $out=Join-Path $qa 'login-gui-stdout.log'
    $err=Join-Path $qa 'login-gui-stderr.log'
    $run=Start-Process -FilePath $exe -ArgumentList @('-test.run=^TestBrowserCancellation$','-test.v','-test.count=1','-test.timeout=45s') -WorkingDirectory $PSScriptRoot -WindowStyle Normal -PassThru -Wait -RedirectStandardOutput $out -RedirectStandardError $err
    Get-Content -LiteralPath $out
    Get-Content -LiteralPath $err
    if($run.ExitCode -ne 0){throw "GUI browser test failed: $($run.ExitCode)"}
} finally {
    $env:YAGO_BROWSER_TEST=$previous
    Pop-Location
}
