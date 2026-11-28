$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'android-env.ps1')
Push-Location $PSScriptRoot
try {
    [System.IO.Directory]::CreateDirectory((Join-Path $PSScriptRoot '.tools/android-build')) | Out-Null
    # Use the real linker executable. The NDK .cmd wrappers have an 8191-character
    # limit, which a Go/C++ link exceeds in this long workspace path.
    $linker = (Join-Path $env:ANDROID_NDK_HOME 'toolchains/llvm/prebuilt/windows-x86_64/bin/clang++.exe').Replace('\','/')
    $env:CGO_LDFLAGS = '-Wl,-z,max-page-size=16384'
    foreach ($target in @(@('arm64','aarch64-linux-android26'),@('arm','armv7a-linux-androideabi26'))) {
        $arch = $target[0]
        Write-Output "Building Android $arch"
        & (Join-Path $env:GOPATH 'bin/gogio.exe') -target android -buildmode archive -arch $arch -minsdk 26 -appid com.yago.mobile -name YaGo -version 0.11.12.40 -tags novulkan -ldflags "-extld=$linker -extldflags=--target=$($target[1])" -o ".tools/android-build/YaGo-$arch.aar" .
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $output = [System.IO.File]::Create((Join-Path $PSScriptRoot '.tools/android-build/YaGo.aar'))
    $combined = New-Object System.IO.Compression.ZipArchive($output,[System.IO.Compression.ZipArchiveMode]::Create)
    $seen = New-Object 'System.Collections.Generic.HashSet[string]'
    try {
        foreach ($arch in @('arm64','arm')) {
            $input = [System.IO.Compression.ZipFile]::OpenRead((Join-Path $PSScriptRoot ".tools/android-build/YaGo-$arch.aar"))
            try {
                foreach ($entry in $input.Entries) {
                    if ($entry.FullName.EndsWith('/') -or -not $seen.Add($entry.FullName)) { continue }
                    $dest = $combined.CreateEntry($entry.FullName,[System.IO.Compression.CompressionLevel]::Optimal)
                    $srcStream = $entry.Open(); $dstStream = $dest.Open()
                    try { $srcStream.CopyTo($dstStream) } finally { $srcStream.Dispose(); $dstStream.Dispose() }
                }
            } finally { $input.Dispose() }
        }
    } finally { $combined.Dispose(); $output.Dispose() }
} finally { Pop-Location }
