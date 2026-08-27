#!/bin/bash

# VKAI Panel Deployment Script
# Usage: ./deploy.sh [environment] [version]

set -e

# Configuration
APP_NAME="vkai-panel"
DEPLOY_DIR="/opt/${APP_NAME}"
RELEASES_DIR="${DEPLOY_DIR}/releases"
CURRENT_LINK="${DEPLOY_DIR}/current"
LOG_DIR="/var/log/${APP_NAME}"
BACKUP_DIR="/opt/${APP_NAME}/backups"
MAX_RELEASES=5

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Logging
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1"
    exit 1
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1"
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        error "Please run as root"
    fi
}

# Create directories
create_directories() {
    log "Creating directories..."
    mkdir -p "${RELEASES_DIR}" "${LOG_DIR}" "${BACKUP_DIR}"
    chown -R vkai:vkai "${DEPLOY_DIR}" "${LOG_DIR}"
}

# Backup database
backup_database() {
    log "Backing up database..."
    BACKUP_FILE="${BACKUP_DIR}/db_$(date +'%Y%m%d_%H%M%S').sql.gz"
    
    if command -v pg_dump &> /dev/null; then
        pg_dump -U vkai -h localhost vkai_panel | gzip > "${BACKUP_FILE}"
        log "Database backup created: ${BACKUP_FILE}"
    else
        warn "pg_dump not found, skipping database backup"
    fi
}

# Deploy release
deploy_release() {
    local RELEASE_FILE=$1
    local RELEASE_ID=$(date +'%Y%m%d_%H%M%S')
    local RELEASE_DIR="${RELEASES_DIR}/${RELEASE_ID}"
    
    log "Deploying release: ${RELEASE_ID}"
    
    # Create release directory
    mkdir -p "${RELEASE_DIR}"
    
    # Extract release
    tar -xzf "${RELEASE_FILE}" -C "${RELEASE_DIR}"
    
    # Set permissions
    chown -R vkai:vkai "${RELEASE_DIR}"
    
    # Update symlink
    ln -sfn "${RELEASE_DIR}" "${CURRENT_LINK}"
    
    log "Release deployed: ${RELEASE_ID}"
}

# Run database migrations
run_migrations() {
    log "Running database migrations..."
    
    if [ -f "${CURRENT_LINK}/api" ]; then
        cd "${CURRENT_LINK}"
        sudo -u vkai ./api migrate up
        log "Migrations completed"
    else
        warn "API binary not found, skipping migrations"
    fi
}

# Restart services
restart_services() {
    log "Restarting services..."
    
    # Restart API
    systemctl restart vkai-panel-api
    log "API service restarted"
    
    # Restart Frontend
    systemctl restart vkai-panel-frontend
    log "Frontend service restarted"
    
    # Wait for services to start
    sleep 5
}

# Health check
health_check() {
    log "Running health check..."
    
    local MAX_RETRIES=10
    local RETRY_COUNT=0
    
    while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
        if curl -f -s http://localhost:30110/health > /dev/null; then
            log "Health check passed"
            return 0
        fi
        
        RETRY_COUNT=$((RETRY_COUNT + 1))
        warn "Health check failed, retrying (${RETRY_COUNT}/${MAX_RETRIES})..."
        sleep 2
    done
    
    error "Health check failed after ${MAX_RETRIES} retries"
}

# Cleanup old releases
cleanup_old_releases() {
    log "Cleaning up old releases..."
    
    local RELEASE_COUNT=$(ls -1d ${RELEASES_DIR}/* 2>/dev/null | wc -l)
    
    if [ $RELEASE_COUNT -gt $MAX_RELEASES ]; then
        local RELEASES_TO_DELETE=$((RELEASE_COUNT - MAX_RELEASES))
        ls -1d ${RELEASES_DIR}/* | head -n $RELEASES_TO_DELETE | xargs rm -rf
        log "Cleaned up ${RELEASES_TO_DELETE} old releases"
    fi
}

# Rollback to previous release
rollback() {
    log "Rolling back to previous release..."
    
    local CURRENT_RELEASE=$(readlink -f "${CURRENT_LINK}")
    local PREVIOUS_RELEASE=$(ls -1d ${RELEASES_DIR}/* | grep -B 1 "${CURRENT_RELEASE}" | head -1)
    
    if [ -z "${PREVIOUS_RELEASE}" ] || [ "${PREVIOUS_RELEASE}" = "${CURRENT_RELEASE}" ]; then
        error "No previous release found"
    fi
    
    log "Rolling back to: $(basename ${PREVIOUS_RELEASE})"
    
    # Update symlink
    ln -sfn "${PREVIOUS_RELEASE}" "${CURRENT_LINK}"
    
    # Restart services
    restart_services
    
    # Health check
    health_check
    
    log "Rollback completed"
}

# Show status
status() {
    echo "=== VKAI Panel Status ==="
    echo ""
    
    # Current release
    if [ -L "${CURRENT_LINK}" ]; then
        echo "Current Release: $(basename $(readlink -f ${CURRENT_LINK}))"
    else
        echo "Current Release: Not deployed"
    fi
    
    echo ""
    
    # Service status
    echo "API Service:"
    systemctl status vkai-panel-api --no-pager | head -5
    echo ""
    
    echo "Frontend Service:"
    systemctl status vkai-panel-frontend --no-pager | head -5
    echo ""
    
    # Disk usage
    echo "Disk Usage:"
    du -sh ${DEPLOY_DIR} 2>/dev/null || echo "N/A"
    echo ""
    
    # Recent logs
    echo "Recent API Logs:"
    journalctl -u vkai-panel-api -n 5 --no-pager
}

# Main
main() {
    local COMMAND=$1
    local ARG1=$2
    
    case "${COMMAND}" in
        deploy)
            check_root
            create_directories
            backup_database
            deploy_release "${ARG1}"
            run_migrations
            restart_services
            health_check
            cleanup_old_releases
            log "Deployment completed successfully!"
            ;;
        rollback)
            check_root
            rollback
            ;;
        status)
            status
            ;;
        restart)
            check_root
            restart_services
            health_check
            log "Services restarted"
            ;;
        *)
            echo "Usage: $0 {deploy|rollback|status|restart} [release-file]"
            echo ""
            echo "Commands:"
            echo "  deploy <file>  - Deploy a new release"
            echo "  rollback       - Rollback to previous release"
            echo "  status         - Show deployment status"
            echo "  restart        - Restart services"
            exit 1
            ;;
    esac
}

main "$@"
