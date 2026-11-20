$ErrorActionPreference = 'Stop'
$tools = Join-Path $PSScriptRoot '.tools'
Add-Type -AssemblyName System.IO.Compression.FileSystem
function Hash-File([string]$file, [string]$kind) {
    $stream = [System.IO.File]::OpenRead($file)
    try { return [System.BitConverter]::ToString([System.Security.Cryptography.HashAlgorithm]::Create($kind).ComputeHash($stream)).Replace('-', '').ToLower() }
    finally { $stream.Dispose() }
}
$archives = @(
    @('ndk.zip','ac5f7762764b1f15341094e148ad4f847d050c38','ndk-unpack'),
    @('platform.zip','0bb560a90a7a2cbd0dd8348224d518b638fe7949','platform-unpack'),
    @('build-tools.zip','af059bb67cf7786f45ee0db85e2d24985df1b4b6','build-tools-unpack'),
    @('platform-tools.zip','b3fde5239ad89229d5a9178375b81c9887a24172','platform-tools-unpack')
)
foreach ($item in $archives) {
    $zip = Join-Path $tools $item[0]
    if ((Hash-File $zip 'SHA1') -ne $item[1]) { throw "Archive checksum mismatch: $zip" }
    $dest = Join-Path $tools $item[2]
    if (-not [System.IO.Directory]::Exists($dest)) {
        Write-Output "Extracting $($item[0])"
        [System.IO.Compression.ZipFile]::ExtractToDirectory($zip, $dest)
    }
}
$sdk = Join-Path $tools 'android-sdk'
foreach ($relative in @('platforms','build-tools','ndk')) { [System.IO.Directory]::CreateDirectory((Join-Path $sdk $relative)) | Out-Null }
$maps = @(@('platform-unpack','platforms/android-35'),@('build-tools-unpack','build-tools/35.0.0'),@('platform-tools-unpack','platform-tools'),@('ndk-unpack','ndk/27.2.12479018'))
foreach ($map in $maps) {
    $dest = Join-Path $sdk $map[1]
    if (-not [System.IO.Directory]::Exists($dest)) {
        $dirs = [System.IO.Directory]::GetDirectories((Join-Path $tools $map[0]))
        if ($dirs.Length -ne 1) { throw "Unexpected archive layout: $($map[0])" }
        [System.IO.Directory]::Move($dirs[0], $dest)
    }
}
if (-not [System.IO.Directory]::Exists((Join-Path $tools 'jdk'))) {
    Write-Output 'Extracting JDK'
    [System.IO.Compression.ZipFile]::ExtractToDirectory((Join-Path $tools 'jdk.zip'), (Join-Path $tools 'jdk'))
}
Write-Output 'Android SDK, NDK and JDK extracted inside .tools.'
