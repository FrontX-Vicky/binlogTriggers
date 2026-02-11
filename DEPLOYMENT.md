# CDC Platform Deployment Guide

## Deployment Options

This project supports multiple deployment methods:
1. **Docker Compose** (Recommended for development/testing)
2. **Systemd Services** (Recommended for production on Linux)
3. **Manual Binary Execution**
4. **Kubernetes** (for large-scale deployments)

---

## Option 1: Docker Compose Deployment

### Prerequisites
- Docker and Docker Compose installed
- MySQL installed and running on host with binlog enabled
- Redis installed and running on host

### Quick Start

1. **Configure environment variables:**
   ```bash
   cp .env.example .env
   # Edit .env with your MySQL and Redis settings (use 127.0.0.1 for host services)
   ```

2. **Build and start services:**
   ```bash
   make docker-build
   make docker-up
   ```

3. **View logs:**
   ```bash
   make docker-logs
   # Or for specific services:
   docker-compose logs -f emitter
   docker-compose logs -f subscribers
   ```

4. **Stop services:**
   ```bash
   make docker-down
   ```

### Docker Compose Services

- **emitter**: CDC emitter reading MySQL binlog (connects to host MySQL)
- **subscribers**: Multi-subscriber processor (connects to host Redis)

**Note:** The containers use `network_mode: host` to access MySQL and Redis running on your host machine.

---

## Option 2: Systemd Services (Linux Production)

### Prerequisites
- Linux server (Ubuntu/Debian/RHEL/CentOS)
- MySQL with binlog enabled
- Redis installed and running
- Go 1.21+ (for building)

### Installation Steps

1. **Build binaries:**
   ```bash
   make build
   ```

2. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env with production values
   ```

3. **Install systemd services:**
   ```bash
   make install-systemd
   ```

4. **Enable and start services:**
   ```bash
   sudo systemctl enable cdc-emitter
   sudo systemctl enable cdc-subscribers
   sudo systemctl start cdc-emitter
   sudo systemctl start cdc-subscribers
   ```

5. **Check status:**
   ```bash
   make status-systemd
   # Or manually:
   sudo systemctl status cdc-emitter
   sudo systemctl status cdc-subscribers
   ```

6. **View logs:**
   ```bash
   sudo journalctl -u cdc-emitter -f
   sudo journalctl -u cdc-subscribers -f
   ```

### Service Management

```bash
# Start services
make start-systemd

# Stop services
make stop-systemd

# Restart services
sudo systemctl restart cdc-emitter
sudo systemctl restart cdc-subscribers

# Check status
make status-systemd
```

---

## Option 3: Manual Binary Execution

### For Development and Testing

1. **Build binaries:**
   ```bash
   make build
   ```

2. **Run emitter (in one terminal):**
   ```bash
   make run-emitter
   ```

3. **Run subscribers (in another terminal):**
   ```bash
   make run-subscribers
   ```

4. **Or run single console subscriber:**
   ```bash
   make run-console
   ```

---

## Configuration

### Emitter Configuration (.env)

```env
# MySQL Connection
DB_USER=cdcuser
DB_PASS=cdcpass
DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=testdb
SERVER_ID=100

# Redis Connection
REDIS_ADDR=127.0.0.1:6379
REDIS_PASS=
REDIS_DB=0
REDIS_CHANNEL=binlog:all
```

### Subscriber Configuration (.env.lead_events)

```env
SUBSCRIBER_NAME=lead_events_processor
REDIS_ADDR=127.0.0.1:6379
REDIS_CHANNEL=binlog:all

# Filters
FILTER_DBS=testdb
FILTER_TABLES=leads
FILTER_OPS=insert,update

# API webhook (optional)
API_URL=http://localhost:8080/webhooks/lead-events
API_TIMEOUT=5s
API_DEBOUNCE=1s
```

### Adding New Subscribers

1. Create a new `.env.{subscriber_name}` file
2. Configure filters and API endpoint
3. Update `ENV_FILES` in docker-compose.yml or systemd service:
   ```
   ENV_FILES=./.env.lead_events,./.env.user_events,./.env.new_subscriber
   ```
4. Restart the subscribers service

---

## MySQL Binlog Setup

Ensure MySQL has binlog enabled:

```sql
-- Check binlog status
SHOW VARIABLES LIKE 'log_bin';

-- Required settings in my.cnf or my.ini:
[mysqld]
server-id=1
log-bin=mysql-bin
binlog-format=ROW
binlog-row-image=FULL
gtid-mode=ON
enforce-gtid-consistency=ON
```

After configuration changes, restart MySQL.

---

## Redis Setup

Basic Redis installation:

```bash
# Ubuntu/Debian
sudo apt-get install redis-server

# Start Redis
sudo systemctl start redis
sudo systemctl enable redis

# Test connection
redis-cli ping
```

---

## Monitoring and Troubleshooting

### Check Service Health

**Docker:**
```bash
docker-compose ps
docker-compose logs emitter
docker-compose logs subscribers
```

**Systemd:**
```bash
sudo systemctl status cdc-emitter
sudo systemctl status cdc-subscribers
sudo journalctl -u cdc-emitter -n 100
```

### Common Issues

1. **Emitter can't connect to MySQL:**
   - Check MySQL credentials in .env
   - Verify binlog is enabled
   - Check user permissions: `GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'user'@'host';`

2. **Subscribers not receiving events:**
   - Verify Redis connection
   - Check Redis channel name matches
   - Check subscriber filters aren't too restrictive

3. **Service crashes immediately:**
   - Check logs: `journalctl -u service-name -n 50`
   - Verify all required environment variables are set
   - Check MySQL/Redis connectivity

### Performance Tuning

- Adjust Redis connection pool size if handling high throughput
- Configure API_DEBOUNCE to batch API calls
- Use FILTER_IDS for specific row filtering
- Monitor memory usage and adjust Docker limits if needed

---

## Security Best Practices

1. **Use strong passwords** for MySQL and Redis
2. **Run services as non-root user** (systemd does this automatically)
3. **Use TLS/SSL** for MySQL and Redis connections in production
4. **Restrict network access** using firewall rules
5. **Keep environment files secure** with proper permissions:
   ```bash
   chmod 600 .env*
   ```
6. **Regularly update** dependencies and images

---

## Scaling

For high-throughput scenarios:

1. **Multiple Subscribers:** Run multiple subscriber instances with different filters
2. **Redis Cluster:** Use Redis cluster for high availability
3. **Load Balancing:** Distribute API webhook calls
4. **Kubernetes:** Deploy using Kubernetes for orchestration (coming soon)

---

## Backup and Recovery

### Backup Considerations

- **MySQL Binlog Position:** Emitter tracks position automatically
- **State Recovery:** Emitter resumes from last position on restart
- **Redis:** Data is transient (pub/sub), no backup needed

### Disaster Recovery

1. Ensure MySQL binlog retention is sufficient
2. Monitor emitter position lag
3. Test recovery procedures regularly

---

## Support

For issues or questions:
- Check logs first
- Review this documentation
- Check GitHub issues: https://github.com/FrontX-Vicky/binlogTriggers
