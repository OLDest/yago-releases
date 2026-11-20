$ErrorActionPreference = 'Stop'
$root = [System.IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\')
$targets = @('.tools/ndk.zip','.tools/platform.zip','.tools/build-tools.zip','.tools/platform-tools.zip','.tools/jdk.zip','.tools/YaGo-phone-preview.exe')
$total = 0L
$inventory = @()
foreach ($relative in $targets) {
    $target = [System.IO.Path]::GetFullPath((Join-Path $root $relative))
    if (-not $target.StartsWith($root + '\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe cleanup target' }
    if ([System.IO.File]::Exists($target)) {
        $size = ([System.IO.FileInfo]$target).Length
        $inventory += [PSCustomObject]@{Relative=$relative;Target=$target;Bytes=$size}
        $total += $size
    }
}
$inventory | Select-Object Relative,Bytes | Format-Table -AutoSize
foreach ($entry in $inventory) { Remove-Item -LiteralPath $entry.Target -Force }
foreach ($name in @('YaGo.exe','YaGo-android.apk','YaGo-android.apk.sha256','.tools/android-signing/yago-test.keystore')) {
    if (-not [System.IO.File]::Exists((Join-Path $root $name))) { throw "Required artifact missing: $name" }
}
$message = "Deleted $($inventory.Count) reproducible archives/QA files; reclaimed $total bytes, permanently (no recycle bin). Preserved APK, Windows exe, sources, hashes, SDK/NDK/JDK, module/build caches, AARs, application state and signing key."
Write-Output $message
[System.IO.File]::WriteAllText((Join-Path $root '.tools/android-cleanup.txt'),$message)
