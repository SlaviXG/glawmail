#!/bin/bash
# GlawMail management script

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
        # Build
        echo "Building..."
        go build -o glawmail ./cmd/glawmail

        # Install service
        echo "Installing service..."
        sudo cp glawmail.service /etc/systemd/system/
        sudo systemctl daemon-reload
        sudo systemctl enable glawmail

        echo "Done! Run: glawmail up"
        ;;
    *)
        echo "Usage: glawmail {up|down|restart|status|logs|install}"
        exit 1
        ;;
esac
