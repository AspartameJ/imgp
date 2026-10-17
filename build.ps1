param(
    [string]$Version = "dev"
)

$ldflags = "-X 'gitcode.com/DonaldTom/imgp/internal/version.Version=$Version'"

New-Item -ItemType Directory -Path "bin" -Force | Out-Null

# Preserve any caller-provided cross-compile environment.
$savedEnv = @{}
foreach ($name in @("GOOS", "GOARCH", "CGO_ENABLED")) {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}

$failed = $false
try {
    $name = "imgp.exe"
    Write-Host "Building $name ..." -ForegroundColor Green
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = 0
    go build -ldflags $ldflags -o "bin/$name" .
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  FAILED" -ForegroundColor Red
        $failed = $true
    } else {
        $size = (Get-Item "bin/$name").Length
        Write-Host "  OK  $([math]::Round($size/1KB, 1)) KB" -ForegroundColor Cyan
    }
}
finally {
    foreach ($name in @("GOOS", "GOARCH", "CGO_ENABLED")) {
        if ($null -eq $savedEnv[$name]) {
            Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
        }
        else {
            Set-Item "Env:\$name" $savedEnv[$name]
        }
    }
}

if ($failed) {
    exit 1
}

Write-Host "`nDone. Binary in ./bin/imgp.exe" -ForegroundColor Green
