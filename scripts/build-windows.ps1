param(
    [ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64',
    [string]$Version = '',
    [switch]$SkipFrontend,
    [switch]$Dev
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
$oldEnvironment = @{}
foreach ($key in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'CGO_CFLAGS', 'CGO_LDFLAGS')) {
    $oldEnvironment[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}
function Check-Exit([string]$step) {
    if ($LASTEXITCODE -ne 0) { throw "$step failed ($LASTEXITCODE)" }
}
try {
    if (!(Test-Path third_party/mihomo/go.mod)) { throw 'Run git submodule update --init --recursive first.' }
    if (!$Version) {
        $Version = git describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -ne 0 -or !$Version) { $Version = 'dev' }
    }
    $coreVersion = git -C third_party/mihomo rev-parse --short HEAD
    Check-Exit 'Read mihomo revision'
    $stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    if (!$SkipFrontend) {
        pnpm --dir frontend install --frozen-lockfile
        Check-Exit 'Install frontend dependencies'
        pnpm --dir frontend run build
        Check-Exit 'Build frontend'
    }
    $env:CGO_ENABLED = '0'
    $env:CGO_CFLAGS = ''
    $env:CGO_LDFLAGS = ''
    # Generate resources on the host, before setting the target architecture.
    $env:GOOS = go env GOHOSTOS
    $env:GOARCH = go env GOHOSTARCH
    go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch $Arch --out rsrc
    Check-Exit 'Generate Windows resources'
    $env:GOOS = 'windows'
    $env:GOARCH = $Arch
    $flags = "-s -w -H windowsgui -X main.version=$Version -X github.com/localhost-copilot/clashcube/internal/updatesig.Stamp=$stamp -X github.com/metacubex/mihomo/constant.Version=alpha-$coreVersion"
    $tags = 'with_gvisor,production'
    if ($Dev) { $tags = 'with_gvisor' }
    $output = "bin/clashcube-windows-$Arch.exe"
    go build -tags $tags -trimpath -buildvcs=false -ldflags $flags -o $output .
    Check-Exit 'Build Windows executable'
    Write-Host "Built $output"
} finally {
    foreach ($key in $oldEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($key, $oldEnvironment[$key], 'Process')
    }
    Pop-Location
}
