$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'android-env.ps1')
$sdkTools = Join-Path $env:ANDROID_HOME 'build-tools/35.0.0'
$apk = Join-Path $PSScriptRoot 'YaGo-android.apk'
& (Join-Path $env:JAVA_HOME 'bin/java.exe') -jar (Join-Path $sdkTools 'lib/apksigner.jar') verify --verbose --print-certs $apk
if ($LASTEXITCODE -ne 0) { throw 'Signature check failed' }
& (Join-Path $sdkTools 'aapt2.exe') dump badging $apk
if ($LASTEXITCODE -ne 0) { throw 'Manifest check failed' }
$llvm = Join-Path $env:ANDROID_NDK_HOME 'toolchains/llvm/prebuilt/windows-x86_64/bin'
foreach ($abi in @('arm64-v8a','armeabi-v7a')) {
    $so = Join-Path $PSScriptRoot ".tools/android-build/aar/jni/$abi/libgio.so"
    $needed = & (Join-Path $llvm 'llvm-readelf.exe') -d $so
    if ($LASTEXITCODE -ne 0) { throw 'ELF check failed' }
    Write-Output "Native dependencies: $abi"
    $needed | Select-String 'NEEDED'
    $symbols = & (Join-Path $llvm 'llvm-nm.exe') -D --defined-only $so
    if (-not ($symbols | Select-String 'Java_com_yago_mobile_Bridge_nativeCommand')) { throw "JNI callback missing: $abi" }
    Write-Output "JNI callback verified: $abi"
    & (Join-Path $llvm 'llvm-readelf.exe') -l $so | Select-String 'LOAD|Align'
}
$stream = [System.IO.File]::OpenRead($apk)
try { $hash = [System.BitConverter]::ToString([System.Security.Cryptography.SHA256]::Create().ComputeHash($stream)).Replace('-', '').ToLower() } finally { $stream.Dispose() }
[System.IO.File]::WriteAllText((Join-Path $PSScriptRoot 'YaGo-android.apk.sha256'), "$hash  YaGo-android.apk`n")
Write-Output "APK bytes: $(([System.IO.FileInfo]$apk).Length)"
Write-Output "APK SHA256: $hash"
