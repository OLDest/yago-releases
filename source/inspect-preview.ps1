$ErrorActionPreference = 'Stop'
$expected = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '.tools/YaGo-phone-preview.exe'))
foreach ($process in [System.Diagnostics.Process]::GetProcessesByName('YaGo-phone-preview')) {
    try {
        $actual = $process.MainModule.FileName
        if ([string]::Equals($actual,$expected,[System.StringComparison]::OrdinalIgnoreCase)) { Write-Output "$($process.Id) $actual" }
    } catch { Write-Output 'Preview process metadata unavailable; left untouched.' }
}
