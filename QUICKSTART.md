# Quick Start Guide - Native Installation

**For Ubuntu 22.04 LTS - 5 Minute Setup**

---

## Prerequisites Check

Your server already has:
- ✅ Go 1.25.1
- ✅ MySQL 8.0.45 (binlog enabled)
- ✅ Redis 8.2.1
- ✅ Git 2.34.1

---

## Installation Steps

### 1. Clone Repository

```bash
cd /var/www/go-workspace
git clone https://github.com/FrontX-Vicky/binlogTriggers.git mysql_changelog_publisher
cd mysql_changelog_publisher
```

### 2. Configure Environment

**Edit .env (emitter configuration):**
```bash
vim .env
```

```env
# MySQL Connection
DB_USER=cdc_user
DB_PASS=your_password
DB_HOST=localhost
DB_PORT=3306
DB_NAME=your_database_name

# CDC Emitter Server ID (must be unique!)
SERVER_ID=2223

# Redis
REDIS_ADDR=127.0.0.1:6379
REDIS_PASS=
REDIS_DB=0
REDIS_CHANNEL=binlog:all
```

**Edit .env.lead_events:**
```bash
vim .env.lead_events
```

```env
SUBSCRIBER_NAME=lead_events
FILTER_TABLES=inquiry_cron
FILTER_OPS=insert
API_URL=https://your-api-endpoint.com/webhook
DEBOUNCE_SECONDS=5
```

### 3. Run Installation Script

```bash
chmod +x scripts/install.sh
./scripts/install.sh
```

This will:
- Build binaries (cdc-emitter, cdc-subscribers)
- Create logs directory
- Install systemd services
- Set permissions
- Create symlinks to /usr/local/bin/ (so you can run commands globally)

### 4. Start Services

```bash
# Enable services (start on boot)
sudo systemctl enable cdc-emitter cdc-subscribers

# Start services
sudo systemctl start cdc-emitter cdc-subscribers

# Check status
sudo systemctl status cdc-emitter
sudo systemctl status cdc-subscribers
```

### 5. Verify

```bash
# View logs in real-time
sudo journalctl -u cdc-emitter -u cdc-subscribers -f

# Test with MySQL insert
mysql -h localhost -u cdc_user -p your_database_name
INSERT INTO inquiry_cron (column1) VALUES ('test');

# Check subscriber processed it
sudo journalctl -u cdc-subscribers -n 20
```

---

## Common Commands

### Service Management
```bash
# Start
sudo systemctl start cdc-emitter cdc-subscribers

# Stop
sudo systemctl stop cdc-emitter cdc-subscribers

# Restart
sudo systemctl restart cdc-emitter cdc-subscribers

# Status
sudo systemctl status cdc-emitter cdc-subscribers
```

### View Logs
```bash
# Real-time logs
sudo journalctl -u cdc-emitter -f
sudo journalctl -u cdc-subscribers -f

# Last 100 lines
sudo journalctl -u cdc-emitter -n 100
sudo journalctl -u cdc-subscribers -n 100

# Errors only
sudo journalctl -u cdc-emitter -p err
```

### Update Code
```bash
cd /var/www/go-workspace/mysql_changelog_publisher
git pull
go build -o cdc-emitter ./cmd/emitter
go build -o cdc-subscribers ./cmd/subscribers
sudo systemctl restart cdc-emitter cdc-subscribers

# Symlinks automatically point to updated binaries!
```

---

## Troubleshooting

### Emitter won't start
```bash
# Check MySQL connection
mysql -h localhost -u cdc_user -p -e "SHOW DATABASES;"

# Check binlog enabled
mysql -h localhost -u cdc_user -p -e "SHOW VARIABLES LIKE 'log_bin';"

# View error logs
sudo journalctl -u cdc-emitter -n 50
```

### Subscribers not receiving events
```bash
# Check Redis
redis-cli PING

# Subscribe to channel manually
redis-cli SUBSCRIBE binlog:all

# Check if emitter is publishing
sudo journalctl -u cdc-emitter | grep -i "published"
```

### API calls not working
```bash
# Check API logs
tail -f logs/lead_events/api_calls.log

# Test API manually
curl -X POST https://your-api-endpoint.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"test":"data"}'
```

---

## Architecture

```
MySQL (Native) → CDC Emitter → Redis Pub/Sub → Subscribers → API/Console
    ↓               ↓              ↓               ↓
  Binlog      Read & Parse    Broadcast      Filter & Process
```

**Port Usage:**
- MySQL: 3306 (localhost only)
- Redis: 6379 (localhost only)

**Server IDs:**
- SDL Service: 2222 (existing)
- CDC Emitter: 2223 (new - configured in .env)

---

## Performance

**Resources Used:**
- CPU: ~5-10% idle, spikes on events
- Memory: ~50-100MB per service
- Disk: ~10-20MB binaries + logs

**Expected Behavior:**
- No activity when no DB changes
- Sub-second latency from MySQL → API
- Auto-reconnect on network issues

---

## Need Help?

1. Check logs: `sudo journalctl -u cdc-emitter -u cdc-subscribers -n 100`
2. Verify .env files
3. Test MySQL/Redis connectivity
4. Review [INSTALL.md](INSTALL.md) for detailed guide
5. Open issue: https://github.com/FrontX-Vicky/binlogTriggers/issues
