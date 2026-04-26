# Docker Setup for ReMS (Restaurant Management System)

This document provides instructions for building and running the entire ReMS project using Docker and Docker Compose.

## Prerequisites

- Docker 20.10+
- Docker Compose 1.29+
- 4GB minimum RAM available for Docker
- Ports 5432, 6379, 3322, 8080, 8080 (ImmuDB UI) available

## Project Architecture

The Docker setup includes:

- **PostgreSQL 18** - Primary relational database
- **Redis 7.2** - In-memory cache and session store
- **ImmuDB** - Immutable audit log database
- **Backend (Go)** - Fiber web application

All services are containerized and communicate through a Docker network called `rms_network`.

## Quick Start

### 1. Prepare Environment Variables

Copy the environment template:
```bash
cp .env.docker .env
```

Edit `.env` to customize database passwords if needed:
```env
DB_USER=postgres
DB_PASSWORD=root
DB_NAME=restaurant_management_system
REDIS_PASSWORD=roote
IMMUDB_PASSWORD=rooter
```

### 2. Build the Images

Build all images (backend will be built, databases use pre-built images):
```bash
docker-compose build
```

Or rebuild with no cache:
```bash
docker-compose build --no-cache
```

### 3. Start All Services

Start all containers:
```bash
docker-compose up -d
```

View logs:
```bash
docker-compose logs -f
```

View backend logs only:
```bash
docker-compose logs -f backend
```

### 4. Verify Services Are Running

Check service status:
```bash
docker-compose ps
```

Health check results:
```bash
docker-compose exec backend wget --no-verbose --tries=1 --spider http://localhost:8080/health
```

## Service Details

### PostgreSQL
- **Container**: rms_postgres
- **Host**: postgres (internal), localhost (external)
- **Port**: 5432
- **Default Database**: restaurant_management_system
- **Default User**: postgres
- **Default Password**: root
- **Volume**: postgres_data

Connect from host:
```bash
psql -h localhost -U postgres -d restaurant_management_system
```

### Redis
- **Container**: rms_redis
- **Host**: redis (internal), localhost (external)
- **Port**: 6379
- **Default Password**: roote
- **Volume**: redis_data

Connect from host:
```bash
redis-cli -h localhost -a roote
```

### ImmuDB
- **Container**: rms_immudb
- **Host**: immudb (internal), localhost (external)
- **Port**: 3322 (gRPC)
- **Port**: 8080 (Web UI)
- **Default Admin User**: immudb
- **Default Password**: rooter
- **Volume**: immudb_data

Access Web Console: http://localhost:8080

Connect via CLI:
```bash
immudb-cli -s localhost -u immudb -P rooter
```

### Backend Application
- **Container**: rms_backend
- **Port**: 8080
- **Service Dependencies**: postgres, redis, immudb
- **Health Check**: GET /health endpoint

Access: http://localhost:8080

## Common Commands

### View All Logs
```bash
docker-compose logs -f
```

### View Specific Service Logs
```bash
docker-compose logs -f backend
docker-compose logs -f postgres
docker-compose logs -f redis
docker-compose logs -f immudb
```

### Stop All Services
```bash
docker-compose stop
```

### Stop and Remove Containers (keep volumes)
```bash
docker-compose down
```

### Remove Everything Including Volumes
```bash
docker-compose down -v
```

### Rebuild a Specific Service
```bash
docker-compose build --no-cache backend
docker-compose up -d backend
```

### Access Backend Container Shell
```bash
docker-compose exec backend sh
```

### Access PostgreSQL Container
```bash
docker-compose exec postgres psql -U postgres -d restaurant_management_system
```

### Restart All Services
```bash
docker-compose restart
```

### Scale Services (if stateless)
```bash
docker-compose up -d --scale backend=3
```

## Troubleshooting

### Backend Cannot Connect to Database

Check backend logs:
```bash
docker-compose logs backend
```

Verify PostgreSQL is healthy:
```bash
docker-compose ps postgres
docker-compose exec postgres pg_isready
```

Verify database exists:
```bash
docker-compose exec postgres psql -U postgres -l
```

### Redis Connection Issues

Check Redis logs:
```bash
docker-compose logs redis
```

Test Redis connection:
```bash
docker-compose exec redis redis-cli -a roote ping
```

### ImmuDB Issues

Check ImmuDB logs:
```bash
docker-compose logs immudb
```

Test ImmuDB connection:
```bash
docker-compose exec immudb immudb-cli -p 3322 -u immudb -P rooter status
```

### Port Already in Use

If a port is already in use, modify the port mapping in `docker-compose.yml`:
```yaml
ports:
  - "5433:5432"  # Use 5433 on host instead of 5432
```

Then reconnect with the new port.

### Container Exits Immediately

Check logs:
```bash
docker-compose logs backend
```

Verify dependency services are running:
```bash
docker-compose ps
```

### Rebuild Failed

Clear cache and rebuild:
```bash
docker system prune
docker-compose build --no-cache
```

## Performance Tuning

### PostgreSQL Tuning
Add to docker-compose.yml postgres section:
```yaml
command:
  - "postgres"
  - "-c"
  - "shared_buffers=256MB"
  - "-c"
  - "effective_cache_size=1GB"
```

### Redis Tuning
Increase memory limit in docker-compose.yml:
```yaml
deploy:
  resources:
    limits:
      memory: 512M
```

## Security Considerations

⚠️ **IMPORTANT**: The `.env` file contains database passwords and should:
- Never be committed to version control
- Never be shared or exposed
- Be regenerated for production
- Use strong passwords in production

For production deployment:
1. Use strong, unique passwords for all services
2. Enable SSL/TLS for external connections
3. Use Docker secrets or environment variable management
4. Restrict network access to authorized IPs only
5. Implement regular backups for volumes

## Backup and Recovery

### Backup PostgreSQL
```bash
docker-compose exec postgres pg_dump -U postgres restaurant_management_system > backup.sql
```

### Restore PostgreSQL
```bash
docker-compose exec -T postgres psql -U postgres restaurant_management_system < backup.sql
```

### Backup Redis
```bash
docker-compose exec redis redis-cli -a roote --rdb /tmp/redis_backup.rdb
docker cp rms_redis:/tmp/redis_backup.rdb ./redis_backup.rdb
```

### Backup ImmuDB
```bash
docker-compose exec immudb immudb-cli -p 3322 -u immudb -P rooter backup
```

## Development Workflow

### Hot Reload Backend (for development)
The backend volume is mounted as read-write. To enable hot reload, add to backend service:
```yaml
volumes:
  - ./backend:/app
environment:
  - GIN_MODE=debug
```

Then rebuild and restart:
```bash
docker-compose down
docker-compose build --no-cache backend
docker-compose up -d
```

### Database Migrations
Execute migration scripts in PostgreSQL:
```bash
docker-compose exec postgres psql -U postgres restaurant_management_system < migrations.sql
```

## Monitoring

### Resource Usage
```bash
docker stats
```

### Health Status
```bash
docker-compose ps
```

All services have healthchecks enabled. Check status with `docker ps` and look for `(healthy)` or `(unhealthy)`.

## References

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [PostgreSQL Docker Documentation](https://hub.docker.com/_/postgres)
- [Redis Docker Documentation](https://hub.docker.com/_/redis)
- [ImmuDB Documentation](https://docs.immudb.io/)
