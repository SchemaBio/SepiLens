param([string]$Version="0.2.0-rc2", [string]$OutputDir="dist/rc2")
$ErrorActionPreference="Stop"
$root=Split-Path -Parent $PSScriptRoot
$destination=Join-Path $root $OutputDir
New-Item -ItemType Directory -Force -Path $destination | Out-Null
Push-Location $root
$savedGoOS=$env:GOOS;$savedGoArch=$env:GOARCH;$savedCGO=$env:CGO_ENABLED
try {
 foreach($platform in @(@("windows","amd64"),@("linux","amd64"),@("linux","arm64"))){
  $env:CGO_ENABLED="0";$env:GOOS=$platform[0];$env:GOARCH=$platform[1]
  $name="sepilens-$Version-$($platform[0])-$($platform[1])";if($platform[0]-eq"windows"){$name+=".exe"}
  & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $destination $name) ./cmd/sepilens
  if($LASTEXITCODE-ne 0){throw "Build failed"}
 }
 & go list -m -json all | Set-Content -Encoding utf8 (Join-Path $destination "dependencies.json")
 if($LASTEXITCODE-ne 0){throw "Dependency inventory failed"}
 & python (Join-Path $PSScriptRoot "dependencies.py") --out $destination
 if($LASTEXITCODE-ne 0){throw "License inventory failed"}
 Get-ChildItem -LiteralPath $destination -File | Where-Object { $_.Name -eq "dependencies.json" -or $_.Name -match "^sepilens-.*-(windows-amd64\.exe|linux-amd64|linux-arm64)$" } | ForEach-Object { "$((Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLower())  $($_.Name)" } | Set-Content -Encoding ascii (Join-Path $destination "SHA256SUMS")
} finally {$env:GOOS=$savedGoOS;$env:GOARCH=$savedGoArch;$env:CGO_ENABLED=$savedCGO;Pop-Location}
