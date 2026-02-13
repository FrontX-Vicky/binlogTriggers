# Filter Configuration Guide

Complete guide for filtering CDC events.

---

## Filter Types

### 1. Include Filters (Whitelist)
Process **ONLY** events matching these criteria.

### 2. Exclude Filters (Blacklist)  
**REJECT** events matching these criteria (takes precedence over include filters).

---

## Available Filters

### Database Filters

**`FILTER_DBS`** (Include)
- Only process events from specified databases
- Comma-separated list
- Example: `FILTER_DBS=mydb,testdb`

**`EXCLUDE_DBS`** (Exclude)
- Ignore events from specified databases
- Takes precedence over FILTER_DBS
- Example: `EXCLUDE_DBS=mysql,information_schema,performance_schema,sys`

### Table Filters

**`FILTER_TABLES`** (Include)
- Only process events from specified tables
- Comma-separated list
- Example: `FILTER_TABLES=users,orders,inquiry_cron`

**`EXCLUDE_TABLES`** (Exclude)
- Ignore events from specified tables
- Takes precedence over FILTER_TABLES
- Example: `EXCLUDE_TABLES=audit_log,session_cache,temp_data`

### Operation Filters

**`FILTER_OPS`** (Include)
- Only process specific operations
- Values: `insert`, `update`, `delete`
- Comma-separated list
- Example: `FILTER_OPS=insert,update`

### ID Filters

**`FILTER_IDS`** (Include)
- Only process rows with specific primary key values
- Comma-separated list
- Example: `FILTER_IDS=1,2,3,100`

### Column Change Filters

**`FILTER_CHANGE_ANY`** (Include)
- Only process if ANY of the specified columns changed
- Comma-separated list
- Example: `FILTER_CHANGE_ANY=status,email,phone`

**`FILTER_CHANGE_ALL`** (Include)
- Only process if ALL specified columns changed
- Comma-separated list
- Example: `FILTER_CHANGE_ALL=status,updated_at`

---

## Filter Logic

### Execution Order

1. **Exclude filters first** (EXCLUDE_DBS, EXCLUDE_TABLES)
   - If matched → **REJECT event**
   
2. **Include filters** (FILTER_DBS, FILTER_TABLES, etc.)
   - If not matched → **REJECT event**

3. **Pass event** to subscriber

### Rules

- **Exclude takes precedence**: If an event matches exclude filter, it's rejected regardless of include filters
- **Empty filter = match all**: If FILTER_TABLES is empty, all tables pass (unless excluded)
- **Comma-separated**: All filters accept comma-separated lists
- **Case-sensitive**: Table and database names are case-sensitive

---

## Common Use Cases

### Case 1: Process only specific tables

```env
FILTER_TABLES=users,orders
```

**Result**: Only `users` and `orders` table events are processed.

---

### Case 2: Process all tables except audit logs

```env
EXCLUDE_TABLES=audit_log,session_log,activity_log
```

**Result**: All tables except the excluded ones are processed.

---

### Case 3: Process specific table but exclude system databases

```env
FILTER_TABLES=users
EXCLUDE_DBS=mysql,information_schema,performance_schema,sys
```

**Result**: Only `users` table from non-system databases.

---

### Case 4: Process inserts only, excluding temp tables

```env
FILTER_OPS=insert
EXCLUDE_TABLES=temp_cache,temp_session
```

**Result**: Only INSERT operations, excluding temp tables.

---

### Case 5: Monitor specific user changes

```env
FILTER_TABLES=users
FILTER_IDS=1,2,3,100
FILTER_CHANGE_ANY=email,phone,status
```

**Result**: Only users with IDs 1,2,3,100 when email/phone/status changes.

---

### Case 6: Exclude all system tables

```env
EXCLUDE_DBS=mysql,information_schema,performance_schema,sys
EXCLUDE_TABLES=session,cache,temp_%
```

**Note**: Wildcard (`%`) is **NOT supported** currently. You must list exact table names.

---

## Example Configurations

### Lead Events Subscriber

`.env.lead_events`:
```env
SUBSCRIBER_NAME=lead_events
FILTER_TABLES=inquiry_cron
FILTER_OPS=insert
EXCLUDE_DBS=mysql,test
API_URL=https://api.example.com/webhook
DEBOUNCE_SECONDS=5
```

**Behavior**: 
- ✅ Process: INSERT into `inquiry_cron`
- ❌ Reject: Updates/Deletes
- ❌ Reject: Events from `mysql` or `test` databases

---

### User Events Subscriber

`.env.user_events`:
```env
SUBSCRIBER_NAME=user_events
FILTER_TABLES=user,user_profile
FILTER_CHANGE_ANY=email,status,verified
EXCLUDE_TABLES=user_temp,user_cache
```

**Behavior**:
- ✅ Process: Changes to `user` or `user_profile` tables
- ✅ Process: Only if `email`, `status`, or `verified` columns changed
- ❌ Reject: `user_temp` and `user_cache` tables

---

### All Events (Except System)

`.env.all_events`:
```env
SUBSCRIBER_NAME=all_events
EXCLUDE_DBS=mysql,information_schema,performance_schema,sys
EXCLUDE_TABLES=sessions,cache
```

**Behavior**:
- ✅ Process: All user database events
- ❌ Reject: System database events
- ❌ Reject: `sessions` and `cache` tables

---

## Testing Filters

### 1. Check Configuration

```bash
# View current filters
cat .env.lead_events | grep -E "FILTER|EXCLUDE"
```

### 2. Test with Live Data

```bash
# Start subscriber with verbose logging
sudo journalctl -u cdc-subscribers -f

# Insert test data
mysql -h localhost -u cdc_user -p your_db
INSERT INTO inquiry_cron (name) VALUES ('test');

# Check if event was processed
sudo journalctl -u cdc-subscribers -n 20 | grep "event matched"
```

### 3. Debug Filter Issues

```bash
# Check what events are being published
redis-cli SUBSCRIBE binlog:all

# Insert test data and see raw events
```

---

## Advanced Patterns

### Pattern 1: Multi-Database Setup

```env
FILTER_DBS=production_db
FILTER_TABLES=orders,payments
EXCLUDE_TABLES=orders_temp
```

Process orders/payments from production_db only, excluding temp tables.

---

### Pattern 2: Critical Changes Only

```env
FILTER_TABLES=users,accounts
FILTER_OPS=update
FILTER_CHANGE_ANY=password,email,status,role
```

Monitor critical field changes only.

---

### Pattern 3: Exclude High-Volume Tables

```env
# Process everything except high-traffic tables
EXCLUDE_TABLES=page_views,clicks,sessions,visitor_log
```

Prevent subscriber overload from high-volume tables.

---

## Performance Tips

1. **Use EXCLUDE for blacklisting**: Faster than listing all allowed tables
2. **Combine with FILTER_OPS**: Reduce event volume by operation type
3. **Use DEBOUNCE_SECONDS**: Prevent duplicate API calls on rapid updates
4. **Monitor logs**: Check for "event matched" frequency

---

## Troubleshooting

### Events not being processed

**Check 1: Verify filters**
```bash
cat .env.lead_events
```

**Check 2: Test without filters**
```bash
# Temporarily comment out all FILTER_* and EXCLUDE_* lines
sudo systemctl restart cdc-subscribers
```

**Check 3: Check logs**
```bash
sudo journalctl -u cdc-subscribers -n 100 | grep -i "filter\|match"
```

---

### Too many events being processed

**Solution: Add exclude filters**
```env
EXCLUDE_TABLES=audit_log,session,cache,temp_data
```

---

### Exclude not working

**Check precedence**:
- EXCLUDE always wins over FILTER
- Verify exact table names (case-sensitive)
- Check for typos

```bash
# Test exact table name
mysql -h localhost -u cdc_user -p -e "SHOW TABLES LIKE 'inquiry_cron';"
```

---

## Filter Reference Table

| Filter | Type | Purpose | Example |
|--------|------|---------|---------|
| FILTER_DBS | Include | Specific databases | `mydb,testdb` |
| EXCLUDE_DBS | Exclude | Block databases | `mysql,sys` |
| FILTER_TABLES | Include | Specific tables | `users,orders` |
| EXCLUDE_TABLES | Exclude | Block tables | `audit_log,cache` |
| FILTER_OPS | Include | Specific operations | `insert,update` |
| FILTER_IDS | Include | Specific row IDs | `1,2,3` |
| FILTER_CHANGE_ANY | Include | Any column changed | `email,status` |
| FILTER_CHANGE_ALL | Include | All columns changed | `status,updated_at` |

---

## Summary

**Key Points:**
- ✅ Exclude filters take precedence
- ✅ Empty filter = allow all
- ✅ Combine filters for precise control
- ✅ Test with real data
- ✅ Monitor logs for "event matched"
