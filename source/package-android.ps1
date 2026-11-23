$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'android-env.ps1')
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$work = Join-Path $PSScriptRoot '.tools/android-build'
$sdkTools = Join-Path $env:ANDROID_HOME 'build-tools/35.0.0'
$androidJar = Join-Path $env:ANDROID_HOME 'platforms/android-35/android.jar'
$java = Join-Path $env:JAVA_HOME 'bin/java.exe'
$javac = Join-Path $env:JAVA_HOME 'bin/javac.exe'
$jar = Join-Path $env:JAVA_HOME 'bin/jar.exe'
$aar = Join-Path $work 'YaGo.aar'
$unpack = Join-Path $work 'aar'
if (-not [System.IO.Directory]::Exists($unpack)) { [System.IO.Directory]::CreateDirectory($unpack) | Out-Null }
# Extract entries explicitly so the script is repeatable after a native rebuild.
$zip = [System.IO.Compression.ZipFile]::OpenRead($aar)
try {
    foreach ($entry in $zip.Entries) {
        if ($entry.FullName.EndsWith('/')) { continue }
        $dest = [System.IO.Path]::GetFullPath((Join-Path $unpack $entry.FullName))
        if (-not $dest.StartsWith([System.IO.Path]::GetFullPath($unpack) + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe AAR entry' }
        [System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($dest)) | Out-Null
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $dest, $true)
    }
} finally { $zip.Dispose() }
foreach ($dir in @('generated','classes','dex','assets')) { [System.IO.Directory]::CreateDirectory((Join-Path $work $dir)) | Out-Null }
foreach ($file in @('LICENSE','THIRD_PARTY_NOTICES.txt')) { [System.IO.File]::Copy((Join-Path $PSScriptRoot $file),(Join-Path $work "assets/$file"),$true) }
$compiled = Join-Path $work 'resources.zip'
& (Join-Path $sdkTools 'aapt2.exe') compile --dir (Join-Path $PSScriptRoot 'android/res') -o $compiled
if ($LASTEXITCODE -ne 0) { throw 'Resource compilation failed' }
$resources = Join-Path $work 'resources.apk'
& (Join-Path $sdkTools 'aapt2.exe') link -o $resources --manifest (Join-Path $PSScriptRoot 'android/AndroidManifest.xml') -I $androidJar --java (Join-Path $work 'generated') -R $compiled -A (Join-Path $work 'assets') --auto-add-overlay
if ($LASTEXITCODE -ne 0) { throw 'Manifest link failed' }
$sources = @([System.IO.Directory]::GetFiles((Join-Path $PSScriptRoot 'android/java'),'*.java',[System.IO.SearchOption]::AllDirectories))
$sources += [System.IO.Directory]::GetFiles((Join-Path $work 'generated'),'*.java',[System.IO.SearchOption]::AllDirectories)
& $javac -encoding UTF-8 -source 8 -target 8 -Xlint:-options -bootclasspath $androidJar -classpath (Join-Path $unpack 'classes.jar') -d (Join-Path $work 'classes') @sources
if ($LASTEXITCODE -ne 0) { throw 'Java compilation failed' }
$appJar = Join-Path $work 'app.jar'
& $jar cf $appJar -C (Join-Path $work 'classes') .
if ($LASTEXITCODE -ne 0) { throw 'Java archive failed' }
& $java -cp (Join-Path $sdkTools 'lib/d8.jar') com.android.tools.r8.D8 --min-api 26 --lib $androidJar --output (Join-Path $work 'dex') $appJar (Join-Path $unpack 'classes.jar')
if ($LASTEXITCODE -ne 0) { throw 'DEX compilation failed' }
$unsigned = Join-Path $work 'unsigned.apk'
[System.IO.File]::Copy($resources,$unsigned,$true)
$apk = [System.IO.Compression.ZipFile]::Open($unsigned,[System.IO.Compression.ZipArchiveMode]::Update)
try {
    foreach ($dex in [System.IO.Directory]::GetFiles((Join-Path $work 'dex'),'*.dex')) {
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($apk,$dex,[System.IO.Path]::GetFileName($dex),[System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
    }
    foreach ($abi in @('arm64-v8a','armeabi-v7a')) {
        $so = Join-Path $unpack "jni/$abi/libgio.so"
        if (-not [System.IO.File]::Exists($so)) { throw "Native library missing: $abi" }
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($apk,$so,"lib/$abi/libgio.so",[System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
    }
} finally { $apk.Dispose() }
$aligned = Join-Path $work 'aligned.apk'
& (Join-Path $sdkTools 'zipalign.exe') -P 16 -f 4 $unsigned $aligned
if ($LASTEXITCODE -ne 0) { throw 'APK alignment failed' }
$signing = Join-Path $PSScriptRoot '.tools/android-signing'
[System.IO.Directory]::CreateDirectory($signing) | Out-Null
$keystore = Join-Path $signing 'yago-test.keystore'
if (-not [System.IO.File]::Exists($keystore)) {
    & (Join-Path $env:JAVA_HOME 'bin/keytool.exe') -genkeypair -noprompt -keystore $keystore -storepass android -keypass android -alias yago -keyalg RSA -keysize 3072 -validity 10000 -dname 'CN=YaGo Local Test, O=YaGo, C=RU'
    if ($LASTEXITCODE -ne 0) { throw 'Test signing key creation failed' }
}
$output = Join-Path $PSScriptRoot 'YaGo-android.apk'
& $java -jar (Join-Path $sdkTools 'lib/apksigner.jar') sign --ks $keystore --ks-key-alias yago --ks-pass pass:android --key-pass pass:android --out $output $aligned
if ($LASTEXITCODE -ne 0) { throw 'APK signing failed' }
& $java -jar (Join-Path $sdkTools 'lib/apksigner.jar') verify --verbose --print-certs $output
if ($LASTEXITCODE -ne 0) { throw 'APK signature verification failed' }
& (Join-Path $sdkTools 'zipalign.exe') -P 16 -c 4 $output
if ($LASTEXITCODE -ne 0) { throw 'APK alignment verification failed' }
Write-Output "APK ready: $output"
