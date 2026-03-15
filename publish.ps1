$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# ── Configuration ─────────────────────────────────────────────────────────────
$REGISTRY   = "ghcr.io"
$IMAGE_NAME = "notarisj/volt"          # <owner>/<repo> — must match GitHub repo
$BUILDER    = "volt-multiplatform"
$PLATFORMS  = "linux/amd64"
# ─────────────────────────────────────────────────────────────────────────────

# Derive version from the latest git tag, fall back to "dev"
$VERSION = git describe --tags --exact-match 2>$null
if (-not $VERSION) {
    $VERSION = git describe --tags --abbrev=0 2>$null
    if (-not $VERSION) { $VERSION = "dev" }
}

$FULL_IMAGE  = "$REGISTRY/$IMAGE_NAME"
$TAG_VERSION = "${FULL_IMAGE}:$($VERSION.TrimStart('v'))"   # e.g. ghcr.io/notarisj/volt:1.2.3
$TAG_SEMVER  = "${FULL_IMAGE}:$VERSION"                     # e.g. ghcr.io/notarisj/volt:v1.2.3
$TAG_LATEST  = "${FULL_IMAGE}:latest"

Write-Host "`nVolt — local multi-platform publish" -ForegroundColor Cyan
Write-Host "  Image   : $FULL_IMAGE"
Write-Host "  Version : $VERSION"
Write-Host "  Tags    : $TAG_VERSION, $TAG_SEMVER, $TAG_LATEST"
Write-Host "  Platforms: $PLATFORMS`n"

# ── 1. Log in to ghcr.io ──────────────────────────────────────────────────────
Write-Host "[1/4] Logging in to $REGISTRY..." -ForegroundColor Yellow
Write-Host '      Enter your GitHub Personal Access Token (needs write:packages scope)'
$TOKEN = Read-Host -AsSecureString "GitHub PAT"
$BSTR  = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($TOKEN)
$PLAIN = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto($BSTR)
[System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($BSTR)

$PLAIN | docker login $REGISTRY -u notarisj --password-stdin
if ($LASTEXITCODE -ne 0) { Write-Error "docker login failed"; exit 1 }
Remove-Variable PLAIN -ErrorAction SilentlyContinue

# ── 2. Ensure a buildx builder with multi-platform support exists ─────────────
Write-Host "`n[2/4] Setting up buildx builder ($BUILDER)..." -ForegroundColor Yellow
$existing = docker buildx ls 2>$null | Select-String $BUILDER
if (-not $existing) {
    docker buildx create --name $BUILDER --driver docker-container --bootstrap
    if ($LASTEXITCODE -ne 0) { Write-Error "buildx create failed"; exit 1 }
}
docker buildx use $BUILDER

# ── 3. Build and push ─────────────────────────────────────────────────────────
Write-Host "`n[3/4] Building and pushing ($PLATFORMS)..." -ForegroundColor Yellow
docker buildx build `
    --platform $PLATFORMS `
    --build-arg VERSION=$VERSION `
    --tag $TAG_VERSION `
    --tag $TAG_SEMVER `
    --tag $TAG_LATEST `
    --push `
    .

if ($LASTEXITCODE -ne 0) { Write-Error "docker buildx build failed"; exit 1 }

# ── 4. Done ───────────────────────────────────────────────────────────────────
Write-Host "`n[4/4] Done!" -ForegroundColor Green
Write-Host "  Pushed: $TAG_VERSION"
Write-Host "  Pushed: $TAG_SEMVER"
Write-Host "  Pushed: $TAG_LATEST"
