#!/bin/bash
set -e

# CDC Platform - Quick Native Installation Script
# For Ubuntu 22.04 LTS

PROJECT_DIR="/var/www/go-workspace/mysql_changelog_publisher"
CURRENT_DIR="$(pwd)"

echo "=== CDC Platform Native Installation ==="
echo ""

# Check if running as root
if [[ $EUID -eq 0 ]]; then
   echo "❌ This script should NOT be run as root"
   echo "Run as regular user with sudo privileges"
   exit 1
fi

# Verify we're in the project directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: Must run from project root directory"
    echo "Expected to find go.mod in current directory"
    exit 1
fi

echo "✓ Running from project directory: $CURRENT_DIR"
echo ""

# Step 1: Build binaries
echo "📦 Building binaries..."
go build -o cdc-emitter ./cmd/emitter || { echo "❌ Failed to build emitter"; exit 1; }
go build -o cdc-subscribers ./cmd/subscribers || { echo "❌ Failed to build subscribers"; exit 1; }
echo "✓ Binaries built successfully"
echo ""

# Step 2: Check configuration files
echo "🔧 Checking configuration..."
if [ ! -f ".env" ]; then
    echo "⚠️  Warning: .env file not found"
    echo "Please create .env with MySQL and Redis configuration"
fi

if [ ! -f ".env.lead_events" ]; then
    echo "⚠️  Warning: .env.lead_events not found"
fi

if [ ! -f ".env.user_events" ]; then
    echo "⚠️  Warning: .env.user_events not found"
fi
echo ""

# Step 3: Create logs directory
echo "📁 Creating logs directory..."
mkdir -p logs
mkdir -p logs/emitter
mkdir -p logs/subscribers

# Create log files with proper permissions
touch logs/emitter/messages.log 2>/dev/null || true
touch logs/subscribers/messages.log 2>/dev/null || true

# Set ownership
sudo chown -R www-data:www-data logs
echo "✓ Logs directory created with proper permissions"
echo ""

# Step 4: Set permissions
echo "🔒 Setting permissions..."
sudo chown -R www-data:www-data $CURRENT_DIR
chmod +x cdc-emitter cdc-subscribers
echo "✓ Permissions set"
echo ""

# Step 5: Install systemd services

echo "⚙️  Installing systemd services..."
if [ -f "systemd/cdc-emitter.service" ] && [ -f "systemd/cdc-subscribers.service" ]; then
    # Update WorkingDirectory in service files to current directory
    sudo cp systemd/cdc-emitter.service /etc/systemd/system/
    sudo cp systemd/cdc-subscribers.service /etc/systemd/system/
    
    # Replace placeholder paths with actual path
    sudo sed -i "s|/var/www/go-workspace/mysql_changelog_publisher|$CURRENT_DIR|g" /etc/systemd/system/cdc-emitter.service
    sudo sed -i "s|/var/www/go-workspace/mysql_changelog_publisher|$CURRENT_DIR|g" /etc/systemd/system/cdc-subscribers.service
    
    echo "✓ Service files installed"
else
    echo "⚠️  Warning: Service files not found in systemd/ directory"
    echo "Services will need to be installed manually"
fi
echo ""

# Step 6: Reload systemd
echo "🔄 Reloading systemd..."
sudo systemctl daemon-reload
echo "✓ Systemd reloaded"
echo ""

# Step 7: Create symlinks (optional but recommended)
echo "🔗 Creating symlinks to /usr/local/bin/ (optional)..."
if sudo ln -sf "$CURRENT_DIR/cdc-emitter" /usr/local/bin/cdc-emitter 2>/dev/null && \
   sudo ln -sf "$CURRENT_DIR/cdc-subscribers" /usr/local/bin/cdc-subscribers 2>/dev/null; then
    echo "✓ Symlinks created: /usr/local/bin/cdc-*"
    echo "  You can now run: cdc-emitter, cdc-subscribers from anywhere"
else
    echo "⚠️  Could not create symlinks (may require sudo)"
fi
echo ""

echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Configure .env files with your MySQL and Redis credentials"
echo "2. Enable services: sudo systemctl enable cdc-emitter cdc-subscribers"
echo "3. Start services: sudo systemctl start cdc-emitter cdc-subscribers"
echo "4. Check status: sudo systemctl status cdc-emitter cdc-subscribers"
echo "5. View logs: sudo journalctl -u cdc-emitter -u cdc-subscribers -f"
echo ""
echo "Configuration files:"
echo "  - Main: $CURRENT_DIR/.env"
echo "  - Lead Events: $CURRENT_DIR/.env.lead_events"
echo "  - User Events: $CURRENT_DIR/.env.user_events"
echo ""
echo "Service files:"
echo "  - /etc/systemd/system/cdc-emitter.service"
echo "  - /etc/systemd/system/cdc-subscribers.service"
echo ""
