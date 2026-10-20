param([string]$Version="0.2.0-rc2", [string]$OutputDir="dist/rc2")
$ErrorActionPreference="Stop"
if($Version -notmatch '^\d+\.\d+\.\d+(-[a-z0-9.]+)?$'){throw "Invalid version"}
$root=Split-Path -Parent $PSScriptRoot
$context=Join-Path $root ("tmp/image-context-"+[guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $context | Out-Null
foreach($arch in @("amd64","arm64")){
 $binary=Join-Path $root "$OutputDir/sepilens-$Version-linux-$arch"
 if(!(Test-Path -LiteralPath $binary)){throw "Run release.ps1 first"}
 Copy-Item -LiteralPath $binary -Destination (Join-Path $context "sepilens-linux-$arch")
}
Copy-Item -LiteralPath (Join-Path $root "LICENSE"),(Join-Path $root "THIRD_PARTY_NOTICES.md") -Destination $context
Copy-Item -LiteralPath (Join-Path $root "internal/rules/svcv4/UPSTREAM_LICENSE") -Destination (Join-Path $context "SVC_UPSTREAM_LICENSE")
@'
FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
RUN apk add --no-cache ca-certificates tzdata && addgroup -S sepilens && adduser -S -G sepilens sepilens
WORKDIR /app
ARG TARGETARCH
COPY sepilens-linux-${TARGETARCH} /app/sepilens
COPY LICENSE THIRD_PARTY_NOTICES.md SVC_UPSTREAM_LICENSE /app/
RUN chmod 755 /app/sepilens && mkdir /app/data && chown sepilens:sepilens /app/data
USER sepilens
VOLUME ["/app/data"]
EXPOSE 8080
ENTRYPOINT ["/app/sepilens"]
CMD ["serve", "-addr", ":8080", "-data-dir", "/app/data"]
'@ | Set-Content -Encoding ascii (Join-Path $context "Dockerfile")
$output=Join-Path $root "$OutputDir/sepilens-$Version-runtime-images.tar"
& docker buildx build --platform linux/amd64,linux/arm64 -t "sepilens:$Version" --output "type=oci,dest=$output" $context
if($LASTEXITCODE-ne 0){throw "Image build failed"}
