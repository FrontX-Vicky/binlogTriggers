# CDC Platform Installation Guide - Ubuntu 22.04 LTS (VM)

Complete step-by-step installation for Ubuntu 22.04.3 LTS (Jammy) on VM.

**Target System:**
- OS: Ubuntu 22.04.3 LTS (Jammy Jellyfish)
- Environment: Virtual Machine
- Architecture: x86_64

---

## Table of Contents
1. [Prerequisites Installation](#prerequisites-installation)
2. [MySQL Setup & Configuration](#mysql-setup--configuration)
3. [Redis Setup](#redis-setup)
4. [Project Setup](#project-setup)
5. [Configuration](#configuration)
6. [Build Binaries](#build-binaries)
7. [Systemd Service Setup](#systemd-service-setup)
8. [Verification Tests](#verification-tests)
9. [Troubleshooting](#troubleshooting)

---

## Prerequisites Installation

### System Update & Basic Tools

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install basic development tools
sudo apt install -y curl wget git build-essential vim net-tools

# Install Go 1.23.1 (required for building)
cd /tmp
wget https://go.dev/dl/go1.23.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.1.linux-amd64.tar.gz

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version
# Should output: go version go1.23.1 linux/amd64
```

---

## MySQL Setup & Configuration

### Install MySQL 8.0 on Ubuntu 22.04

```bash
# Install MySQL Server
sudo apt install -y mysql-server mysql-client

# Start MySQL service
sudo systemctl start mysql
sudo systemctl enable mysql

# Check MySQL status
sudo systemctl status mysql

# Secure MySQL installation (set root password, remove test DB, etc.)
sudo mysql_secure_installation
```

### Create CDC User

```bash
# Login to MySQL as root
sudo mysql -u root -p

# Create dedicated user for CDC
CREATE USER 'cdc_user'@'localhost' IDENTIFIED BY 'your_secure_password';
GRANT REPLICATION SLAVE, REPLICATION CLIENT, SELECT ON *.* TO 'cdc_user'@'localhost';
FLUSH PRIVILEGES;
EXIT;
```

### Configure MySQL for Binlog Replication

```bash
# Edit MySQL configuration
sudo vim /etc/mysql/mysql.conf.d/mysqld.cnf

# Add these settings under [mysqld] section:
```

Add to `/etc/mysql/mysql.conf.d/mysqld.cnf`:

```ini
[mysqld]
# Server identification (must be unique if multiple MySQL instances)
server-id=1

# Binary logging configuration
log-bin=/var/log/mysql/mysql-bin
binlog-format=ROW
binlog-row-image=FULL

# Binary log retention (7 days)
binlog_expire_logs_seconds=604800

# Max binlog size (1GB)
max_binlog_size=1073741824
```

**Restart MySQL:**

```bash
sudo systemctl restart mysql
sudo systemctl status mysql

# Verify binlog is enabled
sudo mysql -u root -p -e "SHOW VARIABLES LIKE 'log_bin';"
sudo mysql -u root -p -e "SHOW VARIABLES LIKE 'binlog_format';"
sudo mysql -u root -p -e "SHOW VARIABLES LIKE 'server_id';"

# Check binary logs
sudo mysql -u root -p -e "SHOW BINARY LOGS;"
```

### Test MySQL Connection

```bash
# Test CDC user connection
mysql -h localhost -u cdc_user -p -e "SHOW DATABASES;"

# Verify replication privileges
mysql -h localhost -u cdc_user -p -e "SHOW GRANTS;"
```

---

## Redis Setup

### Install Redis on Ubuntu 22.04

```bash
# Install Redis
sudo apt install -y redis-server

# Configure Redis for production
sudo vim /etc/redis/redis.conf

# Recommended settings:
# supervised systemd
# bind 127.0.0.1 ::1
# protected-mode yes
# maxmemory 256mb
# maxmemory-policy allkeys-lru

# Restart Redis
sudo systemctl restart redis-server
sudo systemctl enable redis-server

# Check Redis status
sudo systemctl status redis-server
```

### Test Redis Connection

```bash
# Test Redis
redis-cli ping
# Should return: PONG

# Check Redis info
redis-cli info server
```

---

## Docker Installation

### Install Docker

**Ubuntu/Debian:**
```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Add current user to docker group
sudo usermod -aG docker $USER

# Install Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# Enable Docker service
sudo systemctl enable docker
sudo systemctl start docker

# Log out and back in for group changes
# Then verify
docker --version
docker-compose --version
```

**Windows Server:**
```powershell
# Install Docker Desktop
# Download from: https://www.docker.com/products/docker-desktop

# Or use Chocolatey
choco install docker-desktop -y

# Restart required
```

---

## Project Setup

### Clone Repository

```bash
# Clone the project
cd /opt  # or your preferred directory
git clone https://github.com/FrontX-Vicky/binlogTriggers.git
cd binlogTriggers

# Or if you have the files locally, copy them
```

**Windows:**
```powershell
cd C:\
git clone https://github.com/FrontX-Vicky/binlogTriggers.git
cd binlogTriggers
```

### Install Go Dependencies

```bash
# Download dependencies
go mod download
go mod verify

# Test build
go build -o test-emitter ./cmd/emitter
rm test-emitter  # or del test-emitter on Windows
```

---

## Configuration

### Create Environment Files

```bash
# Copy example to create main .env
cp .env.example .env

# Create subscriber environment files (examples provided)
# .env.lead_events and .env.user_events should already exist
```

### Configure Emitter (.env)

Edit `.env` file:

```env
# MySQL Connection - Use your actual credentials
DB_USER=cdcuser
DB_PASS=strong_password_here
DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=your_database
SERVER_ID=100

# Redis Connection
REDIS_ADDR=127.0.0.1:6379
REDIS_PASS=
REDIS_DB=0
REDIS_CHANNEL=binlog:all

# Logging
LOG_LEVEL=info
```

### Configure Subscribers

Edit `.env.lead_events`:

```env
SUBSCRIBER_NAME=lead_events_processor
REDIS_ADDR=127.0.0.1:6379
REDIS_PASS=
REDIS_DB=0
REDIS_CHANNEL=binlog:all

# Filter for specific tables
FILTER_DBS=your_database
FILTER_TABLES=leads,contacts
FILTER_OPS=insert,update

# API webhook (optional - configure later)
# API_URL=http://localhost:8080/webhooks/lead-events
# API_TIMEOUT=5s
# API_DEBOUNCE=1s

LOG_LEVEL=info
```

Edit `.env.user_events`:

```env
SUBSCRIBER_NAME=user_events_processor
REDIS_ADDR=127.0.0.1:6379
REDIS_PASS=
REDIS_DB=0
REDIS_CHANNEL=binlog:all

FILTER_DBS=your_database
FILTER_TABLES=users
FILTER_OPS=insert,update,delete

# API_URL=http://localhost:8080/webhooks/user-events
# API_TIMEOUT=5s

LOG_LEVEL=info
```

---

## Pre-Flight Checks

Run these checks before starting the services:

### 1. MySQL Connectivity Test

```bash
# Test MySQL connection with CDC user
mysql -h 127.0.0.1 -u cdcuser -p -e "SHOW DATABASES;"

# Verify binlog settings
mysql -h 127.0.0.1 -u cdcuser -p << EOF
SHOW VARIABLES LIKE 'log_bin';
SHOW VARIABLES LIKE 'binlog_format';
SHOW VARIABLES LIKE 'server_id';
SHOW MASTER STATUS;
EOF
```

**Expected output:**
- `log_bin` = ON
- `binlog_format` = ROW
- `server_id` = 1 (or your configured value)
- `SHOW MASTER STATUS` should show binary log file and position

### 2. Redis Connectivity Test

```bash
# Test Redis
redis-cli ping
# Expected: PONG

# Test pub/sub
redis-cli PUBLISH binlog:all "test message"
# Expected: (integer) 0 or more
```

### 3. Docker Test

```bash
# Test Docker
docker --version
docker-compose --version

# Test Docker is running
docker ps
```

### 4. Network Ports Test

```bash
# Check if required ports are open
# MySQL: 3306
# Redis: 6379

# Linux
sudo netstat -tlnp | grep -E '3306|6379'

# Or use ss
sudo ss -tlnp | grep -E '3306|6379'
```

**Windows PowerShell:**
```powershell
# Check MySQL port
Test-NetConnection -ComputerName 127.0.0.1 -Port 3306

# Check Redis port
Test-NetConnection -ComputerName 127.0.0.1 -Port 6379
```

### 5. Firewall Check

**Linux:**
```bash
# Check firewall status
sudo ufw status

# If needed, allow ports
sudo ufw allow 3306/tcp
sudo ufw allow 6379/tcp
```

**Windows:**
```powershell
# Check firewall rules for MySQL and Redis
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "*MySQL*"}
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "*Redis*"}
```

### 6. Test Build

```bash
# Build binaries to verify everything compiles
make build

# Should create:
# - bin/emitter
# - bin/subscribers
# - bin/console-subscriber
```

---

## Running the Services

You have three options:

### Option A: Docker Compose (Recommended)

```bash
# Build Docker images
make docker-build

# Start services
make docker-up

# Or manually
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f emitter
docker-compose logs -f subscribers
```

### Option B: Systemd Services (Linux Production)

```bash
# Install as systemd services
make install-systemd

# Enable services
sudo systemctl enable cdc-emitter
sudo systemctl enable cdc-subscribers

# Start services
sudo systemctl start cdc-emitter
sudo systemctl start cdc-subscribers

# Check status
sudo systemctl status cdc-emitter
sudo systemctl status cdc-subscribers

# View logs
sudo journalctl -u cdc-emitter -f
sudo journalctl -u cdc-subscribers -f
```

### Option C: Manual Binary Execution (Testing/Development)

```bash
# Terminal 1 - Run emitter
make run-emitter

# Terminal 2 - Run subscribers
make run-subscribers

# Or run console subscriber for testing
make run-console
```

```bash
# Test with inquiry_cron table (lead_events subscriber)
mysql -h localhost -u cdc_user -p your_database_name

# Insert test data
INSERT INTO inquiry_cron (column1, column2) VALUES ('test', 'data');

# Check subscriber logs
sudo journalctl -u cdc-subscribers -n 20

# Check API call logs
tail -f /var/www/go-workspace/mysql_changelog_publisher/logs/lead_events/api_calls.log
```

### Redis Pub/Sub Verification

```bash
# Terminal 1 - Subscribe to channel
redis-cli SUBSCRIBE binlog:all

# Terminal 2 - Insert MySQL data
mysql -h localhost -u cdc_user -p -e "INSERT INTO your_database.test_table (name) VALUES ('Redis Test');"

# Terminal 1 should show the published event
```

### Monitor System Resources

```bash
# Check CPU/Memory usage
htop

# Or
top -c | grep cdc

# Check disk usage
df -h

# Check network connections
sudo netstat -tlnp | grep -E '3306|6379'
```

---

## Troubleshooting

### Emitter Won't Start

```bash
# Check MySQL connection
mysql -h localhost -u cdc_user -p -e "SHOW DATABASES;"

# Check binlog enabled
mysql -h localhost -u cdc_user -p -e "SHOW VARIABLES LIKE 'log_bin';"

# Check permissions
mysql -h localhost -u cdc_user -p -e "SHOW GRANTS;"

# View emitter logs
sudo journalctl -u cdc-emitter -n 100 --no-pager

# Check .env file
cat /var/www/go-workspace/mysql_changelog_publisher/.env
```

### Subscribers Not Receiving Events

```bash
# Check Redis connection
redis-cli PING

# Check if emitter is publishing
redis-cli SUBSCRIBE binlog:all
# (wait for events)

# Check subscriber logs
sudo journalctl -u cdc-subscribers -n 100 --no-pager

# Test manual subscription
redis-cli PSUBSCRIBE 'binlog:*'
```

### High CPU Usage

```bash
# Check binlog size
sudo ls -lh /var/log/mysql/mysql-bin.*

# Check event rate
sudo journalctl -u cdc-emitter --since "1 minute ago" | wc -l

# Adjust binlog retention
sudo mysql -u root -p -e "SET GLOBAL binlog_expire_logs_seconds = 259200;" # 3 days
```

### Service Crashes/Restarts

```bash
# View crash logs
sudo journalctl -u cdc-emitter --since "1 hour ago" | grep -i "error\|fail\|exit"
sudo journalctl -u cdc-subscribers --since "1 hour ago" | grep -i "error\|fail\|exit"

# Check for "invalid sequence" errors (MySQL packet corruption)
sudo journalctl -u cdc-emitter | grep "invalid sequence"

# If frequent crashes, check:
# 1. Network stability to MySQL
# 2. MySQL max_connections setting
# 3. SERVER_ID conflicts (must be unique)
```

### API Calls Not Working

```bash
# Check API URL in .env.lead_events
cat .env.lead_events

# Test API manually
curl -X POST https://your-api-endpoint.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"test":"data"}'

# Check API logs
tail -f logs/lead_events/api_calls.log

# Check debounce setting
grep DEBOUNCE .env.lead_events
```

### Permission Issues

```bash
# Fix ownership
sudo chown -R www-data:www-data /var/www/go-workspace/mysql_changelog_publisher

# Fix binary permissions
chmod +x /var/www/go-workspace/mysql_changelog_publisher/cdc-*

# Fix log directory
mkdir -p /var/www/go-workspace/mysql_changelog_publisher/logs
sudo chown -R www-data:www-data /var/www/go-workspace/mysql_changelog_publisher/logs
```

### Service Management Commands

```bash
# Stop services
sudo systemctl stop cdc-emitter cdc-subscribers

# Restart services
sudo systemctl restart cdc-emitter cdc-subscribers

# View full service config
systemctl cat cdc-emitter
systemctl cat cdc-subscribers

# Reload systemd after config changes
sudo systemctl daemon-reload

# Disable services (prevent auto-start)
sudo systemctl disable cdc-emitter cdc-subscribers
```

---

## Maintenance

### Update Binaries

```bash
cd /var/www/go-workspace/mysql_changelog_publisher

# Pull latest code
git pull origin main

# Rebuild
go build -o cdc-emitter ./cmd/emitter
go build -o cdc-subscribers ./cmd/subscribers

# Restart services
sudo systemctl restart cdc-emitter cdc-subscribers
```

### Backup Configuration

```bash
# Backup env files
cp .env .env.backup
cp .env.lead_events .env.lead_events.backup

# Backup logs (optional)
tar -czf logs-backup-$(date +%Y%m%d).tar.gz logs/
```

### Monitor Logs

```bash
# Real-time monitoring
sudo journalctl -u cdc-emitter -u cdc-subscribers -f

# Export logs
sudo journalctl -u cdc-emitter --since "24 hours ago" > emitter-last24h.log
```

---

## Performance Tuning

### MySQL Optimization

```sql
-- Check binlog cache size
SHOW VARIABLES LIKE 'binlog_cache_size';

-- Increase if needed (in my.cnf)
-- binlog_cache_size = 1M

-- Check connection limits
SHOW VARIABLES LIKE 'max_connections';
```

### Redis Optimization

```bash
# Edit /etc/redis/redis.conf
sudo vim /etc/redis/redis.conf

# Recommended settings for CDC:
# maxmemory 512mb
# maxmemory-policy allkeys-lru
# save ""  # Disable persistence if not needed

sudo systemctl restart redis-server
```

---

## Quick Reference

### Service Commands
```bash
# Start
sudo systemctl start cdc-emitter cdc-subscribers

# Stop
sudo systemctl stop cdc-emitter cdc-subscribers

# Restart
sudo systemctl restart cdc-emitter cdc-subscribers

# Status
sudo systemctl status cdc-emitter cdc-subscribers

# Logs
sudo journalctl -u cdc-emitter -f
sudo journalctl -u cdc-subscribers -f
```

### Build Commands
```bash
# Build both
go build -o cdc-emitter ./cmd/emitter && go build -o cdc-subscribers ./cmd/subscribers

# Build emitter only
go build -o cdc-emitter ./cmd/emitter

# Build subscribers only
go build -o cdc-subscribers ./cmd/subscribers
```

### Configuration Files
- `/var/www/go-workspace/mysql_changelog_publisher/.env` - Emitter config
- `/var/www/go-workspace/mysql_changelog_publisher/.env.lead_events` - Lead events subscriber
- `/var/www/go-workspace/mysql_changelog_publisher/.env.user_events` - User events subscriber
- `/etc/systemd/system/cdc-emitter.service` - Emitter service
- `/etc/systemd/system/cdc-subscribers.service` - Subscribers service

---

## Support

For issues or questions:
1. Check logs: `sudo journalctl -u cdc-emitter -u cdc-subscribers -n 100`
2. Verify configuration files
3. Test MySQL/Redis connectivity
4. Review GitHub issues: https://github.com/FrontX-Vicky/binlogTriggers/issues

```bash
# 1. Monitor subscriber logs
docker-compose logs -f subscribers

# 2. In another terminal, create data in MySQL
mysql -h 127.0.0.1 -u root -p << EOF
USE your_database;
INSERT INTO leads (name, email, status) VALUES ('John Doe', 'john@example.com', 'new');
UPDATE leads SET status = 'contacted' WHERE email = 'john@example.com';
EOF

# 3. Check subscriber logs - should see filtered events
# If API_URL is configured, check if webhooks are being called
```

### Test 7: Health Check Script

Create a test script:

```bash
cat > health_check.sh << 'EOF'
#!/bin/bash

echo "=== CDC Platform Health Check ==="
echo ""

# Check MySQL
echo "1. MySQL Connection:"
mysql -h 127.0.0.1 -u cdcuser -p'your_password' -e "SELECT 1" 2>/dev/null && echo "✓ OK" || echo "✗ FAILED"

# Check Redis
echo "2. Redis Connection:"
redis-cli ping 2>/dev/null && echo "✓ OK" || echo "✗ FAILED"

# Check Docker services
echo "3. Docker Services:"
docker-compose ps | grep "Up" && echo "✓ OK" || echo "✗ FAILED"

# Check emitter logs
echo "4. Emitter Status:"
docker-compose logs emitter | tail -5

# Check subscribers logs
echo "5. Subscribers Status:"
docker-compose logs subscribers | tail -5

echo ""
echo "=== Health Check Complete ==="
EOF

chmod +x health_check.sh
./health_check.sh
```

---

## Troubleshooting

### Common Issues

#### 1. Emitter Can't Connect to MySQL

**Error:** `Error connecting to MySQL` or `Access denied`

**Solution:**
```bash
# Verify credentials
mysql -h 127.0.0.1 -u cdcuser -p

# Grant proper permissions
mysql -u root -p << EOF
GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'cdcuser'@'%';
GRANT SELECT ON your_database.* TO 'cdcuser'@'%';
FLUSH PRIVILEGES;
EOF

# Check MySQL is listening
sudo netstat -tlnp | grep 3306
```

#### 2. Binlog Not Enabled

**Error:** `Binary logging is not enabled`

**Solution:**
```bash
# Edit MySQL config
sudo nano /etc/mysql/mysql.conf.d/mysqld.cnf

# Add under [mysqld]:
log-bin=mysql-bin
binlog-format=ROW
server-id=1

# Restart MySQL
sudo systemctl restart mysql

# Verify
mysql -u root -p -e "SHOW VARIABLES LIKE 'log_bin';"
```

#### 3. Redis Connection Failed

**Error:** `Could not connect to Redis`

**Solution:**
```bash
# Check Redis is running
sudo systemctl status redis
# or
redis-cli ping

# Start Redis if stopped
sudo systemctl start redis

# Check Redis config
sudo nano /etc/redis/redis.conf
# Ensure: bind 127.0.0.1
# Or: bind 0.0.0.0 (if accessing remotely)

# Restart Redis
sudo systemctl restart redis
```

#### 4. Docker Services Not Starting

**Error:** `Container exited with code 1`

**Solution:**
```bash
# Check detailed logs
docker-compose logs emitter
docker-compose logs subscribers

# Verify .env file exists and is correct
cat .env

# Rebuild images
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

#### 5. No Events Being Received

**Check:**
```bash
# 1. Verify emitter is reading binlog
docker-compose logs emitter | grep -i "reading\|position"

# 2. Check if data changes are ROW format
mysql -u root -p -e "SHOW VARIABLES LIKE 'binlog_format';"
# Should be 'ROW'

# 3. Check Redis channel name matches
cat .env | grep REDIS_CHANNEL
cat .env.lead_events | grep REDIS_CHANNEL

# 4. Check subscriber filters aren't too restrictive
cat .env.lead_events
```

#### 6. Permission Denied Errors

**Linux:**
```bash
# Fix ownership
sudo chown -R $USER:$USER /opt/binlogTriggers

# Fix systemd service permissions
sudo chown -R cdc:cdc /opt/cdc-platform
```

#### 7. Port Already in Use

**Solution:**
```bash
# Find what's using the port
sudo lsof -i :3306
sudo lsof -i :6379

# Or with netstat
sudo netstat -tlnp | grep -E '3306|6379'

# Kill the process or change port in config
```

---

## Post-Installation

### 1. Set Up Monitoring

```bash
# Watch logs continuously
docker-compose logs -f

# Monitor resource usage
docker stats
```

### 2. Configure Log Rotation

For systemd services, logs are in journald:
```bash
# Configure journald retention
sudo nano /etc/systemd/journald.conf
# Set: SystemMaxUse=500M

sudo systemctl restart systemd-journald
```

### 3. Set Up Backups

```bash
# Backup MySQL binlog position periodically
mysql -h 127.0.0.1 -u root -p -e "SHOW MASTER STATUS;" > /backup/binlog_position_$(date +%Y%m%d).txt
```

### 4. Production Checklist

- [ ] MySQL binlog enabled and tested
- [ ] Redis running and tested
- [ ] Firewall rules configured
- [ ] CDC user has proper permissions
- [ ] Environment files configured
- [ ] Services running and healthy
- [ ] End-to-end test passed
- [ ] Monitoring set up
- [ ] Log rotation configured
- [ ] Backup strategy in place

---

## Quick Reference Commands

```bash
# Build everything
make build

# Docker operations
make docker-build
make docker-up
make docker-down
make docker-logs

# Systemd operations (Linux)
make install-systemd
make start-systemd
make stop-systemd
make status-systemd

# View logs
docker-compose logs -f emitter
docker-compose logs -f subscribers
sudo journalctl -u cdc-emitter -f
sudo journalctl -u cdc-subscribers -f

# Restart services
docker-compose restart
sudo systemctl restart cdc-emitter cdc-subscribers

# Health check
docker-compose ps
sudo systemctl status cdc-emitter cdc-subscribers
```

---

## Support

If you encounter issues:
1. Check the logs first
2. Review troubleshooting section
3. Verify all pre-flight checks pass
4. Check GitHub issues: https://github.com/FrontX-Vicky/binlogTriggers

---

**Installation complete!** Your CDC platform should now be running and capturing database changes.
