$ErrorActionPreference = "Stop"

$containerName = "volt-container"
$imageName = "volt-app"
$portMapping = "8686:8686"

# Use absolute paths for current directory bindings
$currentDir = Get-Location
$dataDir = "$currentDir\demo\data"
$uploadsDir = "$currentDir\demo\uploads"

Write-Host "Checking if data and uploads directories exist..."
if (!(Test-Path $dataDir)) { New-Item -ItemType Directory -Force -Path $dataDir | Out-Null }
if (!(Test-Path $uploadsDir)) { New-Item -ItemType Directory -Force -Path $uploadsDir | Out-Null }

Write-Host "`n[1/3] Building the Docker image ($imageName)..."
docker build -t $imageName .

Write-Host "`n[2/3] Checking for existing container..."
$containerExists = docker ps -a -q -f name=^/${containerName}$
if ($containerExists) {
    Write-Host "Stopping and removing existing container ($containerName)..."
    docker stop $containerName | Out-Null
    docker rm $containerName | Out-Null
}

Write-Host "`n[3/3] Starting new container..."
docker run -d `
  --name $containerName `
  -p $portMapping `
  -v "${dataDir}:/app/data" `
  -v "${uploadsDir}:/app/uploads" `
  $imageName

Write-Host "`nDone! Volt is running at http://localhost:8686"
