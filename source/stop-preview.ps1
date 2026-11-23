param([Parameter(Mandatory=$true)][int]$PreviewProcessId)
$ErrorActionPreference = 'Stop'
$expected = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '.tools/YaGo-phone-preview.exe'))
$process = [System.Diagnostics.Process]::GetProcessById($PreviewProcessId)
try {
    if (-not [string]::Equals($process.MainModule.FileName,$expected,[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Process is not this project preview' }
    $process.Kill()
    if (-not $process.WaitForExit(5000)) { throw 'Preview did not exit' }
    Write-Output 'Stopped the verified orphaned QA preview. Windows player left running.'
} finally { $process.Dispose() }
