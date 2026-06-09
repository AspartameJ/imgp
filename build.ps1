param(
    [ValidateSet("windows", "linux", "darwin", "all")]
    [string]$Target = "all"
)

$arches = @{
    windows = @("amd64")
    linux   = @("amd64", "arm64")
    darwin  = @("amd64", "arm64")
}

if ($Target -eq "all") {
    $targets = $arches.Keys
} else {
    $targets = @($Target)
}

foreach ($os in $targets) {
    foreach ($arch in $arches[$os]) {
        $ext = if ($os -eq "windows") { ".exe" } else { "" }
        $name = "imgp-$os-$arch$ext"
        Write-Host "Building $name ..." -ForegroundColor Green
        $env:GOOS = $os
        $env:GOARCH = $arch
        $env:CGO_ENABLED = 0
        go build -o "bin/$name" .
        if ($LASTEXITCODE -eq 0) {
            $size = (Get-Item "bin/$name").Length
            Write-Host "  OK  $([math]::Round($size/1KB, 1)) KB" -ForegroundColor Cyan
        } else {
            Write-Host "  FAILED" -ForegroundColor Red
        }
    }
}

Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host "`nDone. Binaries in ./bin/" -ForegroundColor Green
