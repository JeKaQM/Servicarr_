<div align="center">
  <img src="web/static/images/favicon.svg" alt="Servicarr" width="80">
  
# Servicarr

**Live Demo**: [https://status.jenodoescode.com](https://status.jenodoescode.com)
</div>

A lightweight status page application that monitors multiple services and displays their uptime status. Built with Go backend and vanilla JavaScript frontend, containerized with Docker for easy deployment.

## Features

- **Service Monitoring**: Monitor multiple services with configurable health checks
- **Authentication**: Secure login with IP-based rate limiting and blocking
- **Admin Controls**: Enable/disable service monitoring from the web interface
- **Uptime Tracking**: Persistent uptime statistics stored in SQLite
- **Responsive Design**: Mobile-optimized UI that works on all devices
- **Docker Ready**: Pre-configured Docker setup for instant deployment

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Or: Go 1.25+ and a web browser

### Using Docker (Recommended)

1. **Clone the repository**
   ```bash
   git clone https://github.com/JeKaQM/Servicarr_.git
   cd Servicarr_
   ```

2. **Configure services** - Edit `deploy/docker-compose.yml` to add your services:
   ```yaml
   environment:
     SERVICES: "service1:http://localhost:8001,service2:http://localhost:8002"
   ```

3. **Start the application**
   ```bash
   docker-compose -f deploy/docker-compose.yml up -d
   ```

4. **Access the app** - Open http://localhost:3000 in your browser

### Running Locally

1. **Set up environment variables** - Create a `.env` file (see `.env.example`)

2. **Run the application**
   ```bash
   go run ./app
   ```

3. **Access at** http://localhost:3000

## Configuration

Edit the `.env` file to customize:

- `PORT` - Server port (default: 3000)
- `ADMIN_PASSWORD` - Login password
- `SERVICES` - Comma-separated service list (format: `name:url`)
- `INSECURE_DEV` - Set to `true` for development HTTP cookies, `false` for production HTTPS

## Default Credentials

- **Username**: `admin`
- **Password**: Check your `.env` file

## Security Features

- IP-based rate limiting (10 requests/second per IP)
- Automatic IP blocking after 3 failed login attempts (24-hour duration)
- CSRF token protection
- Session-based authentication with 24-hour timeout
- SameSite cookies for modern browsers (iOS Safari compatible)

## Project Structure

```
Serviccarr_/
├── app/main.go           # Go backend server
├── web/
│   ├── static/           # Frontend assets (CSS, JS, images)
│   └── templates/        # HTML templates
├── deploy/
│   ├── Dockerfile        # Container image definition
│   └── docker-compose.yml # Docker Compose configuration
├── go.mod               # Go dependencies
└── .env.example         # Example configuration
```

## API Endpoints

- `GET /` - Main page
- `POST /api/login` - Authenticate
- `POST /api/logout` - End session
- `GET /api/check` - Get current service status
- `POST /api/toggle` - Enable/disable monitoring
- `GET /blocked` - IP blocked page (auto-redirects if blocked)

## Development

### Building the Docker image

```bash
docker build -f deploy/Dockerfile -t your-registry/servicarr:latest .
docker push your-registry/servicarr:latest
```

### Running tests

```bash
go test ./...
```

## Troubleshooting

**App won't start?**
- Check `.env` file exists and is configured
- Ensure port 3000 is not in use
- Check logs for detailed error messages

**Services show as down?**
- Verify service URLs are correct and accessible
- Check firewall/network connectivity
- Ensure services are running and responding to HTTP requests

**Login not working?**
- Clear browser cookies
- Check you're using correct admin password
- If IP blocked, wait 24 hours or restart the app

## Contributing

This repository uses GitHub Actions for continuous integration. Pull requests must pass all status checks before merging:

- Go code formatting and linting
- Build validation
- Test execution
- Docker image build
- Security scanning

See [.github/workflows/README.md](.github/workflows/README.md) for details on the CI/CD pipeline.

## License

MIT

## Author

Created by jekaq
