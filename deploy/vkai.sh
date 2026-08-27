#!/bin/bash
# ============================================================
# vKAI Panel - Service Management Script
# ============================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

VKAI_HOME="/opt/vkai-panel"

print_usage() {
    echo -e "${BLUE}vKAI Panel Management Script${NC}"
    echo ""
    echo "Usage: vkai <command>"
    echo ""
    echo "Commands:"
    echo "  start       Start all services"
    echo "  stop        Stop all services"
    echo "  restart     Restart all services"
    echo "  status      Show service status"
    echo "  logs        View logs (api|frontend|agent)"
    echo "  db          Database operations (backup|restore|console)"
    echo "  ssl         SSL certificate management"
    echo "  update      Update vKAI Panel"
    echo "  uninstall   Uninstall vKAI Panel"
    echo ""
}

start_services() {
    echo -e "${GREEN}Starting vKAI Panel services...${NC}"
    systemctl start vkai-api
    systemctl start vkai-frontend
    systemctl start nginx
    echo -e "${GREEN}All services started${NC}"
    show_status
}

stop_services() {
    echo -e "${YELLOW}Stopping vKAI Panel services...${NC}"
    systemctl stop vkai-frontend
    systemctl stop vkai-api
    echo -e "${YELLOW}All services stopped${NC}"
}

restart_services() {
    echo -e "${YELLOW}Restarting vKAI Panel services...${NC}"
    systemctl restart vkai-api
    systemctl restart vkai-frontend
    systemctl restart nginx
    echo -e "${GREEN}All services restarted${NC}"
    show_status
}

show_status() {
    echo ""
    echo -e "${BLUE}Service Status:${NC}"
    echo "────────────────────────────────────────"
    
    # API Status
    if systemctl is-active --quiet vkai-api; then
        echo -e "  vkai-api:      ${GREEN}● running${NC}"
    else
        echo -e "  vkai-api:      ${RED}● stopped${NC}"
    fi
    
    # Frontend Status
    if systemctl is-active --quiet vkai-frontend; then
        echo -e "  vkai-frontend: ${GREEN}● running${NC}"
    else
        echo -e "  vkai-frontend: ${RED}● stopped${NC}"
    fi
    
    # Nginx Status
    if systemctl is-active --quiet nginx; then
        echo -e "  nginx:         ${GREEN}● running${NC}"
    else
        echo -e "  nginx:         ${RED}● stopped${NC}"
    fi
    
    # PostgreSQL Status
    if systemctl is-active --quiet postgresql; then
        echo -e "  postgresql:    ${GREEN}● running${NC}"
    else
        echo -e "  postgresql:    ${RED}● stopped${NC}"
    fi
    
    # Redis Status
    if systemctl is-active --quiet redis-server; then
        echo -e "  redis:         ${GREEN}● running${NC}"
    else
        echo -e "  redis:         ${RED}● stopped${NC}"
    fi
    
    echo ""
    echo -e "${BLUE}Resource Usage:${NC}"
    echo "────────────────────────────────────────"
    echo "  CPU:    $(top -bn1 | grep "Cpu(s)" | awk '{print $2}')%"
    echo "  Memory: $(free -h | awk '/^Mem:/ {print $3 "/" $2}')"
    echo "  Disk:   $(df -h / | awk 'NR==2 {print $3 "/" $2 " (" $5 ")"}')"
    echo ""
}

view_logs() {
    local service=$1
    case $service in
        api)
            journalctl -u vkai-api -f --no-pager
            ;;
        frontend)
            journalctl -u vkai-frontend -f --no-pager
            ;;
        agent)
            journalctl -u vkai-agent -f --no-pager
            ;;
        *)
            echo -e "${RED}Unknown service: $service${NC}"
            echo "Available: api, frontend, agent"
            exit 1
            ;;
    esac
}

db_operations() {
    local action=$1
    case $action in
        backup)
            local backup_file="/var/backups/vkai/db_$(date +%Y%m%d_%H%M%S).sql.gz"
            echo -e "${GREEN}Backing up database...${NC}"
            sudo -u postgres pg_dump vkai_panel | gzip > "$backup_file"
            echo -e "${GREEN}Backup saved to: $backup_file${NC}"
            ;;
        restore)
            if [ -z "$2" ]; then
                echo -e "${RED}Usage: vkai db restore <backup_file>${NC}"
                exit 1
            fi
            echo -e "${YELLOW}Restoring database from: $2${NC}"
            gunzip -c "$2" | sudo -u postgres psql -d vkai_panel
            echo -e "${GREEN}Database restored${NC}"
            ;;
        console)
            echo -e "${BLUE}Opening PostgreSQL console...${NC}"
            sudo -u postgres psql -d vkai_panel
            ;;
        *)
            echo -e "${RED}Unknown action: $action${NC}"
            echo "Available: backup, restore, console"
            exit 1
            ;;
    esac
}

ssl_operations() {
    local action=$1
    case $action in
        issue)
            if [ -z "$2" ]; then
                echo -e "${RED}Usage: vkai ssl issue <domain>${NC}"
                exit 1
            fi
            echo -e "${GREEN}Issuing SSL certificate for: $2${NC}"
            certbot --nginx -d "$2" --non-interactive --agree-tos --email admin@$2
            ;;
        renew)
            echo -e "${GREEN}Renewing SSL certificates...${NC}"
            certbot renew
            ;;
        list)
            echo -e "${BLUE}SSL Certificates:${NC}"
            certbot certificates
            ;;
        *)
            echo -e "${RED}Unknown action: $action${NC}"
            echo "Available: issue, renew, list"
            exit 1
            ;;
    esac
}

update_panel() {
    echo -e "${YELLOW}Updating vKAI Panel...${NC}"
    
    # Backup current version
    local backup_dir="/var/backups/vkai/update_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$backup_dir"
    cp -r $VKAI_HOME/backend "$backup_dir/"
    cp -r $VKAI_HOME/frontend "$backup_dir/"
    
    # Stop services
    stop_services
    
    # Pull latest code
    cd $VKAI_HOME
    git pull origin main
    
    # Rebuild
    build_backend
    build_frontend
    
    # Start services
    start_services
    
    echo -e "${GREEN}Update complete!${NC}"
}

uninstall_panel() {
    echo -e "${RED}WARNING: This will remove vKAI Panel and all data!${NC}"
    read -p "Are you sure? (yes/no): " confirm
    
    if [ "$confirm" != "yes" ]; then
        echo "Cancelled"
        exit 0
    fi
    
    # Stop services
    stop_services
    
    # Remove systemd services
    systemctl disable vkai-api vkai-frontend
    rm -f /etc/systemd/system/vkai-*.service
    systemctl daemon-reload
    
    # Remove files
    rm -rf $VKAI_HOME
    rm -rf /var/log/vkai
    rm -rf /var/backups/vkai
    
    # Remove user
    userdel -r vkai 2>/dev/null || true
    
    echo -e "${GREEN}vKAI Panel uninstalled${NC}"
}

# Main
case "$1" in
    start)
        start_services
        ;;
    stop)
        stop_services
        ;;
    restart)
        restart_services
        ;;
    status)
        show_status
        ;;
    logs)
        view_logs "$2"
        ;;
    db)
        db_operations "$2" "$3"
        ;;
    ssl)
        ssl_operations "$2" "$3"
        ;;
    update)
        update_panel
        ;;
    uninstall)
        uninstall_panel
        ;;
    *)
        print_usage
        ;;
esac
