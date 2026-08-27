package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

type Router struct {
	engine            *gin.Engine
	authHandler       *AuthHandler
	serverHandler     *ServerHandler
	tenantHandler     *TenantHandler
	userHandler       *UserHandler
	healthHandler     *HealthHandler
	websiteHandler    *WebsiteHandler
	sslHandler        *SSLHandler
	databaseHandler   *DatabaseHandler
	cronHandler       *CronHandler
	firewallHandler   *FirewallHandler
	backupHandler     *BackupHandler
	serviceHandler    *ServiceHandler
	fileManagerHandler *FileManagerHandler
	monitoringHandler *MonitoringHandler
	logHandler        *LogHandler
	notificationHandler *NotificationHandler
	auditHandler      *AuditHandler
	clusterHandler    *ClusterHandler
	phpHandler        *PHPHandler
	dnsHandler        *DNSHandler
	securityHandler   *SecurityHandler
	nodeAppHandler    *NodeAppHandler
	reverseProxyHandler *ReverseProxyHandler
	gitDeploymentHandler *GitDeploymentHandler
	wordpressHandler *WordPressHandler
	wsHandler         *WebSocketHandler
	jobHandler        *JobHandler
	configHandler     *ConfigHandler
	jwtManager        *auth.JWTManager
	logger            *zap.Logger
}

func NewRouter(
	authHandler *AuthHandler,
	serverHandler *ServerHandler,
	tenantHandler *TenantHandler,
	userHandler *UserHandler,
	healthHandler *HealthHandler,
	websiteHandler *WebsiteHandler,
	sslHandler *SSLHandler,
	databaseHandler *DatabaseHandler,
	cronHandler *CronHandler,
	firewallHandler *FirewallHandler,
	backupHandler *BackupHandler,
	serviceHandler *ServiceHandler,
	fileManagerHandler *FileManagerHandler,
	monitoringHandler *MonitoringHandler,
	logHandler *LogHandler,
	notificationHandler *NotificationHandler,
	auditHandler *AuditHandler,
	clusterHandler *ClusterHandler,
	phpHandler *PHPHandler,
	dnsHandler *DNSHandler,
	securityHandler *SecurityHandler,
	nodeAppHandler *NodeAppHandler,
	reverseProxyHandler *ReverseProxyHandler,
	gitDeploymentHandler *GitDeploymentHandler,
	wordpressHandler *WordPressHandler,
	wsHandler *WebSocketHandler,
	jobHandler *JobHandler,
	configHandler *ConfigHandler,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) *Router {
	engine := gin.New()

	return &Router{
		engine:             engine,
		authHandler:        authHandler,
		serverHandler:      serverHandler,
		tenantHandler:      tenantHandler,
		userHandler:        userHandler,
		healthHandler:      healthHandler,
		websiteHandler:     websiteHandler,
		sslHandler:         sslHandler,
		databaseHandler:    databaseHandler,
		cronHandler:        cronHandler,
		firewallHandler:    firewallHandler,
		backupHandler:      backupHandler,
		serviceHandler:     serviceHandler,
		fileManagerHandler: fileManagerHandler,
		monitoringHandler:  monitoringHandler,
		logHandler:         logHandler,
		notificationHandler: notificationHandler,
		auditHandler:       auditHandler,
		clusterHandler:     clusterHandler,
		phpHandler:         phpHandler,
		dnsHandler:         dnsHandler,
		securityHandler:    securityHandler,
		nodeAppHandler:     nodeAppHandler,
		reverseProxyHandler: reverseProxyHandler,
		gitDeploymentHandler: gitDeploymentHandler,
		wordpressHandler:   wordpressHandler,
		wsHandler:          wsHandler,
		jobHandler:         jobHandler,
		configHandler:      configHandler,
		jwtManager:         jwtManager,
		logger:             logger,
	}
}

func (r *Router) Setup() *gin.Engine {
	// Global middleware
	r.engine.Use(middleware.RequestID())
	r.engine.Use(middleware.Logger(r.logger))
	r.engine.Use(middleware.Recovery(r.logger))
	r.engine.Use(middleware.CORS())
	r.engine.Use(middleware.RateLimit())

	// Health endpoints (no auth)
	r.engine.GET("/health", r.healthHandler.Health)
	r.engine.GET("/ready", r.healthHandler.Ready)
	r.engine.GET("/live", r.healthHandler.Live)

	// Monitoring endpoints (no auth)
	r.engine.GET("/system", r.monitoringHandler.GetSystemInfo)
	r.engine.GET("/metrics", r.monitoringHandler.GetMetrics)

	// API v1
	v1 := r.engine.Group("/api/v1")

	// Auth endpoints (no auth required)
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/login", r.authHandler.Login)
		authGroup.POST("/refresh", r.authHandler.Refresh)
	}

	// Protected endpoints
	protected := v1.Group("")
	protected.Use(middleware.AuthRequired(r.jwtManager))
	{
		// Auth
		protected.GET("/auth/me", r.authHandler.Me)
		protected.POST("/auth/logout", r.authHandler.Logout)

		// Tenants
		tenants := protected.Group("/tenants")
		{
			tenants.POST("", r.tenantHandler.Create)
			tenants.GET("", r.tenantHandler.List)
			tenants.GET("/:id", r.tenantHandler.Get)
			tenants.PUT("/:id", r.tenantHandler.Update)
			tenants.DELETE("/:id", r.tenantHandler.Delete)
		}

		// Users
		users := protected.Group("/users")
		{
			users.POST("", r.userHandler.Create)
			users.GET("", r.userHandler.List)
			users.GET("/:id", r.userHandler.Get)
			users.PUT("/:id", r.userHandler.Update)
			users.DELETE("/:id", r.userHandler.Delete)
			users.POST("/:id/change-password", r.userHandler.ChangePassword)
		}

		// Servers
		servers := protected.Group("/servers")
		{
			servers.POST("", r.serverHandler.Create)
			servers.GET("", r.serverHandler.List)
			servers.GET("/:id", r.serverHandler.Get)
			servers.PUT("/:id", r.serverHandler.Update)
			servers.DELETE("/:id", r.serverHandler.Delete)
			servers.GET("/:id/metrics", r.serverHandler.GetMetrics)
		}

		// Websites
		websites := protected.Group("/websites")
		{
			websites.POST("", r.websiteHandler.Create)
			websites.GET("", r.websiteHandler.List)
			websites.GET("/:id", r.websiteHandler.Get)
			websites.PUT("/:id", r.websiteHandler.Update)
			websites.DELETE("/:id", r.websiteHandler.Delete)
			websites.POST("/:id/ssl", r.websiteHandler.EnableSSL)
			websites.POST("/:id/domains", r.websiteHandler.AddDomain)
			websites.GET("/:id/domains", r.websiteHandler.ListDomains)
			websites.DELETE("/:id/domains/:domainId", r.websiteHandler.DeleteDomain)
		}

		// Databases
		databases := protected.Group("/databases")
		{
			databases.POST("/servers", r.databaseHandler.CreateServer)
			databases.GET("/servers", r.databaseHandler.ListServers)
			databases.GET("/servers/:id", r.databaseHandler.GetServer)
			databases.DELETE("/servers/:id", r.databaseHandler.DeleteServer)
			databases.POST("", r.databaseHandler.CreateDatabase)
			databases.GET("", r.databaseHandler.ListDatabases)
			databases.DELETE("/:id", r.databaseHandler.DeleteDatabase)
			databases.POST("/:id/change-password", r.databaseHandler.ChangePassword)
		}

		// DNS
		dns := protected.Group("/dns")
		{
			dns.POST("/zones", r.dnsHandler.CreateZone)
			dns.GET("/zones", r.dnsHandler.ListZones)
			dns.GET("/zones/:id", r.dnsHandler.GetZone)
			dns.PUT("/zones/:id", r.dnsHandler.UpdateZone)
			dns.DELETE("/zones/:id", r.dnsHandler.DeleteZone)

			dns.POST("/zones/:zoneId/records", r.dnsHandler.CreateRecord)
			dns.GET("/zones/:zoneId/records", r.dnsHandler.ListRecords)
			dns.GET("/records/:id", r.dnsHandler.GetRecord)
			dns.PUT("/records/:id", r.dnsHandler.UpdateRecord)
			dns.DELETE("/records/:id", r.dnsHandler.DeleteRecord)
		}

		// SSL
		ssl := protected.Group("/ssl")
		{
			ssl.POST("/letsencrypt", r.sslHandler.IssueLetsEncrypt)
			ssl.POST("/custom", r.sslHandler.UploadCustom)
			ssl.GET("", r.sslHandler.List)
			ssl.GET("/expiring", r.sslHandler.GetExpiringSoon)
			ssl.GET("/:id", r.sslHandler.Get)
			ssl.DELETE("/:id", r.sslHandler.Delete)
			ssl.POST("/renew", r.sslHandler.RenewAll)
		}

		// Docker
		// TODO: Add Docker routes

		// Security
		security := protected.Group("/security")
		{
			security.POST("/scans", r.securityHandler.CreateScan)
			security.GET("/scans", r.securityHandler.ListScans)
			security.GET("/scans/:id", r.securityHandler.GetScan)
			security.DELETE("/scans/:id", r.securityHandler.DeleteScan)

			security.GET("/scans/:scanId/vulnerabilities", r.securityHandler.ListVulnerabilitiesByScan)
			security.GET("/vulnerabilities", r.securityHandler.ListVulnerabilitiesByTenant)
			security.GET("/vulnerabilities/:id", r.securityHandler.GetVulnerability)
			security.PUT("/vulnerabilities/:id", r.securityHandler.UpdateVulnerability)
			security.DELETE("/vulnerabilities/:id", r.securityHandler.DeleteVulnerability)

			security.GET("/scans/:scanId/checks", r.securityHandler.ListChecksByScan)

			security.POST("/policies", r.securityHandler.CreatePolicy)
			security.GET("/policies", r.securityHandler.ListPolicies)
			security.GET("/policies/:id", r.securityHandler.GetPolicy)
			security.PUT("/policies/:id", r.securityHandler.UpdatePolicy)
			security.DELETE("/policies/:id", r.securityHandler.DeletePolicy)
		}

		// Files
		files := protected.Group("/files")
		{
			files.GET("/list", r.fileManagerHandler.ListFiles)
			files.GET("/read", r.fileManagerHandler.ReadFile)
			files.POST("/write", r.fileManagerHandler.WriteFile)
			files.POST("/mkdir", r.fileManagerHandler.CreateDirectory)
			files.POST("/delete", r.fileManagerHandler.Delete)
			files.POST("/rename", r.fileManagerHandler.Rename)
			files.POST("/copy", r.fileManagerHandler.Copy)
			files.POST("/chmod", r.fileManagerHandler.ChangePermissions)
			files.POST("/upload", r.fileManagerHandler.Upload)
			files.GET("/download", r.fileManagerHandler.Download)
			files.GET("/search", r.fileManagerHandler.Search)
			files.GET("/disk-usage", r.fileManagerHandler.GetDiskUsage)
		}

		// Cron
		cron := protected.Group("/cron")
		{
			cron.POST("", r.cronHandler.Create)
			cron.GET("", r.cronHandler.List)
			cron.GET("/:id", r.cronHandler.Get)
			cron.PUT("/:id", r.cronHandler.Update)
			cron.DELETE("/:id", r.cronHandler.Delete)
			cron.POST("/:id/toggle", r.cronHandler.ToggleStatus)
			cron.POST("/:id/run", r.cronHandler.RunNow)
		}

		// Firewall
		firewall := protected.Group("/firewall")
		{
			firewall.POST("", r.firewallHandler.Create)
			firewall.GET("", r.firewallHandler.List)
			firewall.GET("/active", r.firewallHandler.GetActiveRules)
			firewall.GET("/:id", r.firewallHandler.Get)
			firewall.PUT("/:id", r.firewallHandler.Update)
			firewall.DELETE("/:id", r.firewallHandler.Delete)
			firewall.POST("/save", r.firewallHandler.SaveRules)
		}

		// Backups
		backups := protected.Group("/backups")
		{
			backups.POST("/jobs", r.backupHandler.CreateJob)
			backups.GET("/jobs", r.backupHandler.ListJobs)
			backups.GET("/jobs/:id", r.backupHandler.GetJob)
			backups.PUT("/jobs/:id", r.backupHandler.UpdateJob)
			backups.DELETE("/jobs/:id", r.backupHandler.DeleteJob)
			backups.POST("/jobs/:id/run", r.backupHandler.RunBackup)
			backups.GET("/records", r.backupHandler.ListRecords)
			backups.DELETE("/records/:id", r.backupHandler.DeleteRecord)
		}

		// Services (systemd)
		services := protected.Group("/services")
		{
			services.GET("", r.serviceHandler.List)
			services.POST("", r.serviceHandler.Create)
			services.GET("/:name", r.serviceHandler.GetStatus)
			services.DELETE("/:name", r.serviceHandler.Delete)
			services.POST("/:name/start", r.serviceHandler.Start)
			services.POST("/:name/stop", r.serviceHandler.Stop)
			services.POST("/:name/restart", r.serviceHandler.Restart)
			services.POST("/:name/enable", r.serviceHandler.Enable)
			services.POST("/:name/disable", r.serviceHandler.Disable)
			services.GET("/:name/logs", r.serviceHandler.GetLogs)
		}

		// PHP
		php := protected.Group("/php")
		{
			php.POST("/versions", r.phpHandler.CreatePHPVersion)
			php.GET("/versions", r.phpHandler.ListPHPVersions)
			php.GET("/versions/:id", r.phpHandler.GetPHPVersion)
			php.PUT("/versions/:id", r.phpHandler.UpdatePHPVersion)
			php.DELETE("/versions/:id", r.phpHandler.DeletePHPVersion)

			php.POST("/pools", r.phpHandler.CreatePHPPool)
			php.GET("/pools", r.phpHandler.ListPHPPools)
			php.GET("/pools/:id", r.phpHandler.GetPHPPool)
			php.PUT("/pools/:id", r.phpHandler.UpdatePHPPool)
			php.DELETE("/pools/:id", r.phpHandler.DeletePHPPool)

			php.POST("/extensions", r.phpHandler.InstallPHPExtension)
			php.GET("/versions/:phpVersionId/extensions", r.phpHandler.ListPHPExtensions)
			php.PUT("/extensions/:id", r.phpHandler.UpdatePHPExtension)
			php.DELETE("/extensions/:id", r.phpHandler.DeletePHPExtension)

			php.GET("/versions/:phpVersionId/config", r.phpHandler.GetPHPConfig)
			php.PUT("/versions/:phpVersionId/config", r.phpHandler.UpdatePHPConfig)
			php.DELETE("/config/:id", r.phpHandler.DeletePHPConfig)
		}

		// Monitoring
		monitoring := protected.Group("/monitoring")
		{
			monitoring.POST("/servers/:server_id/metrics", r.monitoringHandler.RecordMetric)
			monitoring.GET("/servers/:server_id/metrics", r.monitoringHandler.GetServerMetrics)
			monitoring.GET("/servers/:server_id/metrics/latest", r.monitoringHandler.GetLatestMetric)

			monitoring.POST("/alerts", r.monitoringHandler.CreateAlert)
			monitoring.GET("/alerts", r.monitoringHandler.ListAlerts)
			monitoring.GET("/alerts/:id", r.monitoringHandler.GetAlert)
			monitoring.PUT("/alerts/:id", r.monitoringHandler.UpdateAlert)
			monitoring.DELETE("/alerts/:id", r.monitoringHandler.DeleteAlert)
			monitoring.GET("/alerts/:id/logs", r.monitoringHandler.ListAlertLogs)

			monitoring.POST("/dashboards", r.monitoringHandler.CreateDashboard)
			monitoring.GET("/dashboards", r.monitoringHandler.ListDashboards)
			monitoring.GET("/dashboards/:id", r.monitoringHandler.GetDashboard)
			monitoring.PUT("/dashboards/:id", r.monitoringHandler.UpdateDashboard)
			monitoring.DELETE("/dashboards/:id", r.monitoringHandler.DeleteDashboard)
		}

		// Logs
		logs := protected.Group("/logs")
		{
			logs.POST("/search", r.logHandler.SearchEntries)
			logs.POST("/servers/:server_id/entries", r.logHandler.RecordEntry)
			logs.POST("/cleanup", r.logHandler.CleanupOldEntries)

			logs.POST("/sources", r.logHandler.CreateSource)
			logs.GET("/sources", r.logHandler.ListSources)
			logs.GET("/sources/:id", r.logHandler.GetSource)
			logs.PUT("/sources/:id", r.logHandler.UpdateSource)
			logs.DELETE("/sources/:id", r.logHandler.DeleteSource)

			logs.POST("/rotations", r.logHandler.CreateRotation)
			logs.GET("/rotations", r.logHandler.ListRotations)
			logs.GET("/rotations/:id", r.logHandler.GetRotation)
			logs.PUT("/rotations/:id", r.logHandler.UpdateRotation)
			logs.DELETE("/rotations/:id", r.logHandler.DeleteRotation)
		}

		// Deployments
		// TODO: Add deployment routes

		// Node.js Apps
		nodeApps := protected.Group("/node-apps")
		{
			nodeApps.POST("", r.nodeAppHandler.Create)
			nodeApps.GET("", r.nodeAppHandler.List)
			nodeApps.GET("/:id", r.nodeAppHandler.Get)
			nodeApps.PUT("/:id", r.nodeAppHandler.Update)
			nodeApps.DELETE("/:id", r.nodeAppHandler.Delete)
			nodeApps.POST("/:id/start", r.nodeAppHandler.Start)
			nodeApps.POST("/:id/stop", r.nodeAppHandler.Stop)
			nodeApps.POST("/:id/restart", r.nodeAppHandler.Restart)
			nodeApps.GET("/:id/status", r.nodeAppHandler.GetStatus)
			nodeApps.GET("/:id/logs", r.nodeAppHandler.GetLogs)

			nodeApps.POST("/:id/dependencies", r.nodeAppHandler.CreateDependency)
			nodeApps.GET("/:id/dependencies", r.nodeAppHandler.ListDependencies)
			nodeApps.PUT("/:id/dependencies/:depId", r.nodeAppHandler.UpdateDependency)
			nodeApps.DELETE("/:id/dependencies/:depId", r.nodeAppHandler.DeleteDependency)

			nodeApps.POST("/:id/environments", r.nodeAppHandler.CreateEnvironment)
			nodeApps.GET("/:id/environments", r.nodeAppHandler.ListEnvironments)
			nodeApps.PUT("/:id/environments/:envId", r.nodeAppHandler.UpdateEnvironment)
			nodeApps.DELETE("/:id/environments/:envId", r.nodeAppHandler.DeleteEnvironment)
		}

		// Reverse Proxy
		reverseProxy := protected.Group("/reverse-proxy")
		{
			reverseProxy.POST("", r.reverseProxyHandler.Create)
			reverseProxy.GET("", r.reverseProxyHandler.List)
			reverseProxy.GET("/:id", r.reverseProxyHandler.Get)
			reverseProxy.PUT("/:id", r.reverseProxyHandler.Update)
			reverseProxy.DELETE("/:id", r.reverseProxyHandler.Delete)
			reverseProxy.GET("/server/:server_id", r.reverseProxyHandler.ListByServer)
			reverseProxy.GET("/:id/access-logs", r.reverseProxyHandler.ListAccessLogs)
			reverseProxy.DELETE("/:id/access-logs", r.reverseProxyHandler.ClearAccessLogs)
		}

		// Git Deployments
		gitDeployments := protected.Group("/git-deployments")
		{
			gitDeployments.POST("", r.gitDeploymentHandler.Create)
			gitDeployments.GET("", r.gitDeploymentHandler.List)
			gitDeployments.GET("/:id", r.gitDeploymentHandler.Get)
			gitDeployments.PUT("/:id", r.gitDeploymentHandler.Update)
			gitDeployments.DELETE("/:id", r.gitDeploymentHandler.Delete)
			gitDeployments.GET("/server/:server_id", r.gitDeploymentHandler.ListByServer)
			gitDeployments.POST("/:id/deploy", r.gitDeploymentHandler.Deploy)
			gitDeployments.GET("/:id/logs", r.gitDeploymentHandler.ListDeploymentLogs)
			gitDeployments.DELETE("/:id/logs", r.gitDeploymentHandler.ClearDeploymentLogs)
		}

		// WordPress
		wordpress := protected.Group("/wordpress")
		{
			wordpress.POST("", r.wordpressHandler.Create)
			wordpress.GET("", r.wordpressHandler.List)
			wordpress.GET("/:id", r.wordpressHandler.Get)
			wordpress.PUT("/:id", r.wordpressHandler.Update)
			wordpress.DELETE("/:id", r.wordpressHandler.Delete)
			wordpress.GET("/server/:server_id", r.wordpressHandler.ListByServer)

			wordpress.POST("/:id/plugins", r.wordpressHandler.InstallPlugin)
			wordpress.GET("/:id/plugins", r.wordpressHandler.ListPlugins)
			wordpress.PUT("/:id/plugins/:pluginId", r.wordpressHandler.UpdatePlugin)
			wordpress.DELETE("/:id/plugins/:pluginId", r.wordpressHandler.DeletePlugin)

			wordpress.POST("/:id/themes", r.wordpressHandler.InstallTheme)
			wordpress.GET("/:id/themes", r.wordpressHandler.ListThemes)
			wordpress.PUT("/:id/themes/:themeId", r.wordpressHandler.UpdateTheme)
			wordpress.DELETE("/:id/themes/:themeId", r.wordpressHandler.DeleteTheme)
		}

		// Notifications
		notifications := protected.Group("/notifications")
		{
			notifications.POST("", r.notificationHandler.Create)
			notifications.GET("", r.notificationHandler.List)
			notifications.GET("/:id", r.notificationHandler.Get)
			notifications.PUT("/:id/read", r.notificationHandler.MarkAsRead)
			notifications.PUT("/read-all", r.notificationHandler.MarkAllAsRead)
			notifications.DELETE("/:id", r.notificationHandler.Delete)
			notifications.POST("/cleanup", r.notificationHandler.CleanupOld)

			notifications.POST("/templates", r.notificationHandler.CreateTemplate)
			notifications.GET("/templates", r.notificationHandler.ListTemplates)
			notifications.GET("/templates/:id", r.notificationHandler.GetTemplate)
			notifications.PUT("/templates/:id", r.notificationHandler.UpdateTemplate)
			notifications.DELETE("/templates/:id", r.notificationHandler.DeleteTemplate)

			notifications.POST("/channels", r.notificationHandler.CreateChannel)
			notifications.GET("/channels", r.notificationHandler.ListChannels)
			notifications.GET("/channels/:id", r.notificationHandler.GetChannel)
			notifications.PUT("/channels/:id", r.notificationHandler.UpdateChannel)
			notifications.DELETE("/channels/:id", r.notificationHandler.DeleteChannel)

			notifications.GET("/preferences", r.notificationHandler.GetPreferences)
			notifications.PUT("/preferences", r.notificationHandler.SetPreference)
		}

		// Audit Logs
		audit := protected.Group("/audit")
		{
			audit.GET("/search", r.auditHandler.Search)
			audit.GET("/stats", r.auditHandler.GetStats)
			audit.GET("/:id", r.auditHandler.Get)
			audit.POST("/cleanup", r.auditHandler.CleanupOld)
		}

		// Clusters
		clusters := protected.Group("/clusters")
		{
			clusters.POST("", r.clusterHandler.Create)
			clusters.GET("", r.clusterHandler.List)
			clusters.GET("/:id", r.clusterHandler.Get)
			clusters.PUT("/:id", r.clusterHandler.Update)
			clusters.DELETE("/:id", r.clusterHandler.Delete)

			clusters.POST("/:id/nodes", r.clusterHandler.AddNode)
			clusters.GET("/:id/nodes", r.clusterHandler.ListNodes)
			clusters.PUT("/:id/nodes/:nodeId", r.clusterHandler.UpdateNode)
			clusters.DELETE("/:id/nodes/:nodeId", r.clusterHandler.RemoveNode)
			clusters.POST("/:id/nodes/:nodeId/heartbeat", r.clusterHandler.NodeHeartbeat)
		}

		// Load Balancers
		loadBalancers := protected.Group("/load-balancers")
		{
			loadBalancers.POST("", r.clusterHandler.CreateLoadBalancer)
			loadBalancers.GET("", r.clusterHandler.ListLoadBalancers)
			loadBalancers.GET("/:id", r.clusterHandler.GetLoadBalancer)
			loadBalancers.PUT("/:id", r.clusterHandler.UpdateLoadBalancer)
			loadBalancers.DELETE("/:id", r.clusterHandler.DeleteLoadBalancer)
		}

		// HA Pairs
		haPairs := protected.Group("/ha-pairs")
		{
			haPairs.POST("", r.clusterHandler.CreateHAPair)
			haPairs.GET("", r.clusterHandler.ListHAPairs)
			haPairs.GET("/:id", r.clusterHandler.GetHAPair)
			haPairs.PUT("/:id", r.clusterHandler.UpdateHAPair)
			haPairs.POST("/:id/failover", r.clusterHandler.TriggerFailover)
			haPairs.DELETE("/:id", r.clusterHandler.DeleteHAPair)
		}

		// WebSocket endpoints
		ws := protected.Group("/ws")
		{
			ws.GET("", r.wsHandler.HandleConnection)
			ws.GET("/status", r.wsHandler.GetStatus)
			ws.GET("/rooms/:room/status", r.wsHandler.GetRoomStatus)
			ws.POST("/broadcast", r.wsHandler.BroadcastMessage)
			ws.POST("/direct", r.wsHandler.SendDirectMessage)
		}

		// Jobs
		jobs := protected.Group("/jobs")
		{
			jobs.GET("", r.jobHandler.ListJobs)
			jobs.GET("/stats", r.jobHandler.GetJobStats)
			jobs.GET("/queue-stats", r.jobHandler.GetQueueStats)
			jobs.GET("/:id", r.jobHandler.GetJob)
			jobs.DELETE("/:id", r.jobHandler.DeleteJob)
			jobs.POST("/:id/cancel", r.jobHandler.CancelJob)
			jobs.POST("/:id/retry", r.jobHandler.RetryJob)
			jobs.POST("/backup", r.jobHandler.EnqueueBackup)
			jobs.POST("/restore", r.jobHandler.EnqueueRestore)
			jobs.POST("/deploy", r.jobHandler.EnqueueDeploy)
			jobs.POST("/ssl", r.jobHandler.EnqueueSSL)
			jobs.POST("/cleanup", r.jobHandler.EnqueueCleanup)
			jobs.POST("/cleanup-old", r.jobHandler.CleanupOldJobs)
		}

		// Config
		config := protected.Group("/config")
		{
			config.POST("/snapshots", r.configHandler.CreateSnapshot)
			config.GET("/snapshots", r.configHandler.ListSnapshots)
			config.GET("/snapshots/:id", r.configHandler.GetSnapshot)
			config.DELETE("/snapshots/:id", r.configHandler.DeleteSnapshot)
			config.POST("/rollback", r.configHandler.Rollback)
			config.GET("/diff", r.configHandler.GetDiff)
			config.GET("/history", r.configHandler.GetSnapshotHistory)
			config.GET("/stats", r.configHandler.GetConfigStats)
			config.POST("/cleanup", r.configHandler.CleanupOldSnapshots)
			config.POST("/validate", r.configHandler.ValidateConfig)

			config.POST("/templates", r.configHandler.CreateTemplate)
			config.GET("/templates", r.configHandler.ListTemplates)
			config.GET("/templates/:id", r.configHandler.GetTemplate)
			config.PUT("/templates/:id", r.configHandler.UpdateTemplate)
			config.DELETE("/templates/:id", r.configHandler.DeleteTemplate)
		}
	}

	// Agent endpoints (agent auth required)
	agent := v1.Group("/agent")
	{
		agent.POST("/heartbeat", func(c *gin.Context) {
			// TODO: Agent heartbeat handler
		})
		agent.POST("/register", func(c *gin.Context) {
			// TODO: Agent registration handler
		})
	}

	return r.engine
}

func (r *Router) Engine() *gin.Engine {
	return r.engine
}
