# CDC Platform Installation Guide - Fresh Server Setup

Complete step-by-step installation for a new server.

---

## Table of Contents
1. [Prerequisites Installation](#prerequisites-installation)
2. [MySQL Setup & Configuration](#mysql-setup--configuration)
3. [Redis Setup](#redis-setup)
4. [Docker Installation](#docker-installation)
5. [Project Setup](#project-setup)
6. [Configuration](#configuration)
7. [Pre-Flight Checks](#pre-flight-checks)
8. [Running the Services](#running-the-services)
9. [Verification Tests](#verification-tests)
10. [Troubleshooting](#troubleshooting)

---

## Prerequisites Installation

### For Linux (Ubuntu/Debian)

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install basic tools
sudo apt install -y curl wget git build-essential

# Install Go (required for building binaries)
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version
```

### For Windows Server

```powershell
# Install Chocolatey (package manager)
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Install Git
choco install git -y

# Install Go
choco install golang -y

# Refresh environment
refreshenv

# Verify Go installation
go version
```

---

## MySQL Setup & Configuration

### Install MySQL

**Ubuntu/Debian:**
```bash
sudo apt install -y mysql-server mysql-client

# Secure MySQL installation
sudo mysql_secure_installation
```

**Windows Server:**
```powershell
# Download MySQL from https://dev.mysql.com/downloads/installer/
# Or use Chocolatey
choco install mysql -y
```

### Configure MySQL for Binlog

**Linux:** Edit `/etc/mysql/mysql.conf.d/mysqld.cnf` or `/etc/my.cnf`

**Windows:** Edit `C:\ProgramData\MySQL\MySQL Server 8.0\my.ini`

Add these settings under `[mysqld]` section:

```ini
[mysqld]
# Server identification
server-id=1

# Binary logging
log-bin=mysql-bin
binlog-format=ROW
binlog-row-image=FULL

# GTID mode (recommended)
gtid-mode=ON
enforce-gtid-consistency=ON

# Binary log retention (7 days)
binlog_expire_logs_seconds=604800
```

### Restart MySQL

**Linux:**
```bash
sudo systemctl restart mysql
sudo systemctl status mysql
```

**Windows (PowerShell as Admin):**
```powershell
Restart-Service MySQL80
Get-Service MySQL80
```

### Create Database User

```bash
# Connect to MySQL
mysql -u root -p

# Or on Windows
mysql -u root -p
```

```sql
-- Create database
CREATE DATABASE IF NOT EXISTS your_database;

-- Create CDC user with replication privileges
CREATE USER 'cdcuser'@'%' IDENTIFIED BY 'strong_password_here';
GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'cdcuser'@'%';
GRANT SELECT ON your_database.* TO 'cdcuser'@'%';
FLUSH PRIVILEGES;

-- Verify binlog is enabled
SHOW VARIABLES LIKE 'log_bin';
SHOW VARIABLES LIKE 'binlog_format';
SHOW VARIABLES LIKE 'server_id';

-- Check binary logs
SHOW BINARY LOGS;

-- Exit
EXIT;
```

### Test MySQL Connection

```bash
# Test connection
mysql -h 127.0.0.1 -u cdcuser -p -e "SHOW DATABASES;"
```

---

## Redis Setup

### Install Redis

**Ubuntu/Debian:**
```bash
sudo apt install -y redis-server

# Configure Redis (optional)
sudo nano /etc/redis/redis.conf
# Set: bind 127.0.0.1
# Set: protected-mode yes

# Restart Redis
sudo systemctl restart redis
sudo systemctl enable redis
sudo systemctl status redis
```

**Windows Server:**
```powershell
# Download from https://github.com/microsoftarchive/redis/releases
# Or use Chocolatey
choco install redis-64 -y

# Start Redis service
redis-server --service-start
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

---

## Verification Tests

### Test 1: Check Services Are Running

**Docker:**
```bash
docker-compose ps
# Both services should be "Up"
```

**Systemd:**
```bash
sudo systemctl status cdc-emitter cdc-subscribers
# Both should be "active (running)"
```

### Test 2: Check Logs

**Docker:**
```bash
# Check emitter connected to MySQL
docker-compose logs emitter | grep -i "connected\|mysql"

# Check emitter connected to Redis
docker-compose logs emitter | grep -i "redis"

# Check subscribers connected
docker-compose logs subscribers | grep -i "connected\|subscribed"
```

**Systemd:**
```bash
sudo journalctl -u cdc-emitter -n 50 | grep -i "connected"
sudo journalctl -u cdc-subscribers -n 50 | grep -i "subscribed"
```

### Test 3: Create Test Data in MySQL

```bash
mysql -h 127.0.0.1 -u root -p
```

```sql
USE your_database;

-- Create test table if doesn't exist
CREATE TABLE IF NOT EXISTS test_table (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert test record
INSERT INTO test_table (name, email) VALUES ('Test User', 'test@example.com');

-- Update test record
UPDATE test_table SET name = 'Updated User' WHERE id = 1;

-- Check the record
SELECT * FROM test_table;
```

### Test 4: Verify Events Are Being Captured

**Docker:**
```bash
# Check emitter logs for published events
docker-compose logs emitter | tail -20

# Should see lines like:
# Published event to binlog:all
# RowEvent{Op:insert, Schema:your_database, Table:test_table}
```

**Console subscriber test:**
```bash
# Run console subscriber to see events in real-time
make run-console

# In another terminal, insert/update MySQL data
# You should see events printed to console
```

### Test 5: Redis Pub/Sub Verification

```bash
# Terminal 1 - Subscribe to channel
redis-cli SUBSCRIBE binlog:all

# Terminal 2 - Perform MySQL operation
mysql -h 127.0.0.1 -u root -p -e "INSERT INTO your_database.test_table (name, email) VALUES ('Redis Test', 'redis@test.com');"

# Terminal 1 should show the message published by emitter
```

### Test 6: End-to-End Test

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
