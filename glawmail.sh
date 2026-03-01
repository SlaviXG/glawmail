#!/bin/bash
# GlawMail management script

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

case "$1" in
    up|start)
        sudo systemctl start glawmail
        echo "GlawMail started"
        ;;
    down|stop)
        sudo systemctl stop glawmail
        echo "GlawMail stopped"
        ;;
    restart)
        sudo systemctl restart glawmail
        echo "GlawMail restarted"
        ;;
    status)
        sudo systemctl status glawmail
        ;;
    logs)
        sudo journalctl -u glawmail -f
        ;;
    install)
        echo "Building..."
        go build -o glawmail ./cmd/glawmail

        echo "Installing service..."

        # Update service file with actual path and user
        sed "s|/home/pi/glawmail|$SCRIPT_DIR|g; s|User=pi|User=$USER|g" glawmail.service | sudo tee /etc/systemd/system/glawmail.service > /dev/null

        sudo systemctl daemon-reload
        sudo systemctl enable glawmail

        echo "Install complete. Run: glawmail up"
        ;;
    uninstall)
        echo "Stopping..."
        sudo systemctl stop glawmail 2>/dev/null
        echo "Disabling auto-start..."
        sudo systemctl disable glawmail 2>/dev/null
        sudo rm -f /etc/systemd/system/glawmail.service
        sudo systemctl daemon-reload
        echo "Uninstalled"
        ;;
    *)
        echo "Usage: glawmail {up|down|restart|status|logs|install|uninstall}"
        exit 1
        ;;
esac
