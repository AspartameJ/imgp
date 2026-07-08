param(
    [string]$Version = "dev"
)

$ldflags = "-X 'internal/version.Version=$Version'"

New-Item -ItemType Directory -Path "bin" -Force | Out-Null

$name = "imgp.exe"
Write-Host "Building $name ..." -ForegroundColor Green
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = 0
go build -ldflags $ldflags -o "bin/$name" .
if ($LASTEXITCODE -ne 0) {
    Write-Host "  FAILED" -ForegroundColor Red
    exit 1
}
$size = (Get-Item "bin/$name").Length
Write-Host "  OK  $([math]::Round($size/1KB, 1)) KB" -ForegroundColor Cyan

Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host "`nDone. Binary in ./bin/imgp.exe" -ForegroundColor Green
