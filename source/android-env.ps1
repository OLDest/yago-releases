$ErrorActionPreference = 'Stop'
$env:GOROOT = Join-Path $PSScriptRoot '.tools/go'
& (Join-Path $PSScriptRoot 'prepare-toolchain.ps1') -ToolchainRoot $env:GOROOT
$env:GOPATH = Join-Path $PSScriptRoot '.tools/gopath'
$env:GOCACHE = Join-Path $PSScriptRoot '.tools/build-cache'
$env:ANDROID_HOME = Join-Path $PSScriptRoot '.tools/android-sdk'
$env:ANDROID_SDK_ROOT = $env:ANDROID_HOME
$env:ANDROID_NDK_HOME = Join-Path $env:ANDROID_HOME 'ndk/27.2.12479018'
$jdkDirs = [System.IO.Directory]::GetDirectories((Join-Path $PSScriptRoot '.tools/jdk'))
if ($jdkDirs.Length -ne 1) { throw 'Expected one local JDK directory' }
$env:JAVA_HOME = $jdkDirs[0]
$env:PATH = (Join-Path $env:GOROOT 'bin') + ';' + (Join-Path $env:JAVA_HOME 'bin') + ';' + (Join-Path $env:GOPATH 'bin') + ';' + $env:PATH
$env:CGO_ENABLED = '1'
$env:GOFLAGS = '-buildvcs=false'
$env:GOTMPDIR = Join-Path $PSScriptRoot '.tools/go-tmp'
$env:TEMP = Join-Path $PSScriptRoot '.tools/tmp'
$env:TMP = $env:TEMP
[System.IO.Directory]::CreateDirectory($env:GOTMPDIR) | Out-Null
[System.IO.Directory]::CreateDirectory($env:TEMP) | Out-Null
