$ErrorActionPreference = 'Stop'
$root = [System.IO.Path]::GetFullPath($PSScriptRoot).TrimEnd('\')
$parent = [System.IO.Directory]::GetParent($root).FullName
$reference = Join-Path $root 'reference'
[System.IO.Directory]::CreateDirectory($reference) | Out-Null
# Preserve all reference material from the preceding analysis in this chat folder.
$inputs = @('article.html','source-tree.json','go-releases.json','review-api.rs','review-auth.rs','review-client.rs','review-config.rs','review-images.rs','review-main.rs','review-net.rs','review-player.rs','review-secret.rs','review-win.rs')
foreach ($name in $inputs) {
    $source = [System.IO.Path]::GetFullPath((Join-Path $parent $name))
    $destination = [System.IO.Path]::GetFullPath((Join-Path $reference $name))
    if (-not $source.StartsWith($parent + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe reference source' }
    if (-not $destination.StartsWith($root + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe reference destination' }
    if ([System.IO.File]::Exists($source) -and -not [System.IO.File]::Exists($destination)) { Move-Item -LiteralPath $source -Destination $destination }
}
# Explicit reproducible targets; toolchain, module cache, source and exe stay.
$targets = @('go-toolchain.zip','.tools/check-linux-music','.tools/check-linux-audio','.tools/check-darwin-music','.tools/check-darwin-audio')
$total = 0L
foreach ($relative in $targets) {
    $target = [System.IO.Path]::GetFullPath((Join-Path $root $relative))
    if (-not $target.StartsWith($root + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe cleanup target' }
    if ([System.IO.File]::Exists($target)) {
        $size = ([System.IO.FileInfo]$target).Length
        Write-Output "$relative : $size bytes"
        $total += $size
        Remove-Item -LiteralPath $target -Force
    }
}
if (-not [System.IO.File]::Exists((Join-Path $root 'YaGo.exe'))) { throw 'Executable missing' }
Write-Output "Reclaimed $total bytes; permanent deletion, no recycle bin. Source, local toolchain, module/build caches, state and executable retained."
Get-ChildItem -LiteralPath $root -File | Select-Object Name,Length
