# Update script for statusapp
# This script pulls the latest image and restarts the container
# while preserving all data in the volume

Write-Host "Pulling latest statusapp image..." -ForegroundColor Cyan
docker-compose pull

Write-Host "`nStopping and removing old container..." -ForegroundColor Cyan
docker-compose down

Write-Host "`nStarting updated container..." -ForegroundColor Cyan
docker-compose up -d

Write-Host "`nVerifying container is running..." -ForegroundColor Cyan
docker-compose ps

Write-Host "`nUpdate complete! Your data has been preserved in the 'statusapp_data' volume." -ForegroundColor Green
Write-Host "Access your app at: http://localhost:4555" -ForegroundColor Yellow
