package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type Router struct {
	engine                *gin.Engine
	authHandler           *AuthHandler
	serverHandler         *ServerHandler
	tenantHandler         *TenantHandler
	userHandler           *UserHandler
	healthHandler         *HealthHandler
	websiteHandler        *WebsiteHandler
	sslHandler            *SSLHandler
	databaseHandler       *DatabaseHandler
	cronHandler           *CronHandler
	firewallHandler       *FirewallHandler
	backupHandler         *BackupHandler
	serviceHandler        *ServiceHandler
	fileManagerHandler    *FileManagerHandler
	monitoringHandler     *MonitoringHandler
	logHandler            *LogHandler
	notificationHandler   *NotificationHandler
	auditHandler          *AuditHandler
	clusterHandler        *ClusterHandler
	phpHandler            *PHPHandler
	dnsHandler            *DNSHandler
	securityHandler       *SecurityHandler
	nodeAppHandler        *NodeAppHandler
	reverseProxyHandler   *ReverseProxyHandler
	gitDeploymentHandler  *GitDeploymentHandler
	wordpressHandler      *WordPressHandler
	wsHandler             *WebSocketHandler
	jobHandler            *JobHandler
	configHandler         *ConfigHandler
	dockerHandler         *DockerHandler
	apiKeyHandler         *APIKeyHandler
	wafHandler            *WAFHandler
	websiteStatsHandler   *WebsiteStatsHandler
	emailMarketingHandler *EmailMarketingHandler
	mailServerHandler     *MailServerHandler
	fileProtectionHandler *FileProtectionHandler
	multiUserHandler      *MultiUserHandler
	dailyReportHandler    *DailyReportHandler
	scheduledTaskHandler  *ScheduledTaskHandler
	tamperProofHandler    *TamperProofHandler
	panelSettingsHandler  *PanelSettingsHandler
	jwtManager            *auth.JWTManager
	logger                *zap.Logger
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
	dockerHandler *DockerHandler,
	apiKeyHandler *APIKeyHandler,
	wafHandler *WAFHandler,
	websiteStatsHandler *WebsiteStatsHandler,
	emailMarketingHandler *EmailMarketingHandler,
	mailServerHandler *MailServerHandler,
	fileProtectionHandler *FileProtectionHandler,
	multiUserHandler *MultiUserHandler,
	dailyReportHandler *DailyReportHandler,
	scheduledTaskHandler *ScheduledTaskHandler,
	tamperProofHandler *TamperProofHandler,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) *Router {
	engine := gin.New()

	r := &Router{
		engine:                engine,
		authHandler:           authHandler,
		serverHandler:         serverHandler,
		tenantHandler:         tenantHandler,
		userHandler:           userHandler,
		healthHandler:         healthHandler,
		websiteHandler:        websiteHandler,
		sslHandler:            sslHandler,
		databaseHandler:       databaseHandler,
		cronHandler:           cronHandler,
		firewallHandler:       firewallHandler,
		backupHandler:         backupHandler,
		serviceHandler:        serviceHandler,
		fileManagerHandler:    fileManagerHandler,
		monitoringHandler:     monitoringHandler,
		logHandler:            logHandler,
		notificationHandler:   notificationHandler,
		auditHandler:          auditHandler,
		clusterHandler:        clusterHandler,
		phpHandler:            phpHandler,
		dnsHandler:            dnsHandler,
		securityHandler:       securityHandler,
		nodeAppHandler:        nodeAppHandler,
		reverseProxyHandler:   reverseProxyHandler,
		gitDeploymentHandler:  gitDeploymentHandler,
		wordpressHandler:      wordpressHandler,
		wsHandler:             wsHandler,
		jobHandler:            jobHandler,
		configHandler:         configHandler,
		dockerHandler:         dockerHandler,
		apiKeyHandler:         apiKeyHandler,
		wafHandler:            wafHandler,
		websiteStatsHandler:   websiteStatsHandler,
		emailMarketingHandler: emailMarketingHandler,
		mailServerHandler:     mailServerHandler,
		fileProtectionHandler: fileProtectionHandler,
		multiUserHandler:      multiUserHandler,
		dailyReportHandler:    dailyReportHandler,
		scheduledTaskHandler:  scheduledTaskHandler,
		tamperProofHandler:    tamperProofHandler,
		jwtManager:            jwtManager,
		logger:                logger,
	}

	// The panel access settings handler is built here rather than being passed
	// in: the API entry point calls NewRouter positionally, so growing the
	// parameter list would break every caller for a dependency the router can
	// resolve on its own.
	r.panelSettingsHandler = newPanelSettingsHandler(auditHandler, logger)

	return r
}

// newPanelSettingsHandler loads the panel access configuration this process is
// serving and wires it to the audit trail. A configuration that cannot be read
// is not fatal - every other route still works - so the failure is logged and
// the settings routes answer 503 instead.
func newPanelSettingsHandler(auditHandler *AuditHandler, logger *zap.Logger) *PanelSettingsHandler {
	panelCfg, err := config.LoadPanelAccess()
	if err != nil {
		if logger != nil {
			logger.Error("panel access settings unavailable: cannot load configuration", zap.Error(err))
		}
		return NewPanelSettingsHandler(nil, logger)
	}

	var auditService *service.AuditService
	if auditHandler != nil {
		auditService = auditHandler.Service()
	}

	return NewPanelSettingsHandler(
		service.NewPanelSettingsService(panelCfg, auditService, logger),
		logger,
	)
}

func (r *Router) Setup() *gin.Engine {
	// Cap multipart buffering so a large upload cannot be held entirely in RAM.
	r.engine.MaxMultipartMemory = 8 << 20

	// Global middleware
	r.engine.Use(middleware.RequestID())
	r.engine.Use(middleware.Logger(r.logger))
	r.engine.Use(middleware.Recovery(r.logger))
	r.engine.Use(middleware.SecurityHeaders())
	r.engine.Use(middleware.CORS())
	r.engine.Use(middleware.RateLimit())

	// Health endpoints (no auth). These report up/down only.
	r.engine.GET("/health", r.healthHandler.Health)
	r.engine.GET("/ready", r.healthHandler.Ready)
	r.engine.GET("/live", r.healthHandler.Live)

	// API v1
	v1 := r.engine.Group("/api/v1")

	// Auth endpoints (no auth required, tight rate limit)
	authGroup := v1.Group("/auth")
	authGroup.Use(middleware.AuthRateLimit())
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

		// Runtime introspection. Previously served unauthenticated at the root,
		// where it fingerprinted the build and leaked load characteristics.
		protected.GET("/system", middleware.RequireAdmin(), r.monitoringHandler.GetSystemInfo)
		protected.GET("/metrics", middleware.RequireAdmin(), r.monitoringHandler.GetMetrics)

		// Tenants
		tenants := protected.Group("/tenants", middleware.RequireAdmin())
		{
			tenants.POST("", r.tenantHandler.Create)
			tenants.GET("", r.tenantHandler.List)
			tenants.GET("/:id", r.tenantHandler.Get)
			tenants.PUT("/:id", r.tenantHandler.Update)
			tenants.DELETE("/:id", r.tenantHandler.Delete)
		}

		// Users
		users := protected.Group("/users", middleware.RequirePermission("user"))
		{
			users.POST("", r.userHandler.Create)
			users.GET("", r.userHandler.List)
			users.GET("/:id", r.userHandler.Get)
			users.PUT("/:id", r.userHandler.Update)
			users.DELETE("/:id", r.userHandler.Delete)
			users.POST("/:id/change-password", r.userHandler.ChangePassword)
		}

		// Servers
		servers := protected.Group("/servers", middleware.RequirePermission("server"))
		{
			servers.POST("", r.serverHandler.Create)
			servers.GET("", r.serverHandler.List)
			servers.GET("/:id", r.serverHandler.Get)
			servers.PUT("/:id", r.serverHandler.Update)
			servers.DELETE("/:id", r.serverHandler.Delete)
			servers.GET("/:id/metrics", r.serverHandler.GetMetrics)
		}

		// Websites
		websites := protected.Group("/websites", middleware.RequirePermission("website"))
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
		databases := protected.Group("/databases", middleware.RequirePermission("database"))
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
		dns := protected.Group("/dns", middleware.RequirePermission("dns"))
		{
			dns.POST("/zones", r.dnsHandler.CreateZone)
			dns.GET("/zones", r.dnsHandler.ListZones)
			dns.GET("/zones/:id", r.dnsHandler.GetZone)
			dns.PUT("/zones/:id", r.dnsHandler.UpdateZone)
			dns.DELETE("/zones/:id", r.dnsHandler.DeleteZone)

			dns.POST("/zones/:id/records", r.dnsHandler.CreateRecord)
			dns.GET("/zones/:id/records", r.dnsHandler.ListRecords)
			dns.GET("/records/:id", r.dnsHandler.GetRecord)
			dns.PUT("/records/:id", r.dnsHandler.UpdateRecord)
			dns.DELETE("/records/:id", r.dnsHandler.DeleteRecord)
		}

		// SSL
		ssl := protected.Group("/ssl", middleware.RequirePermission("ssl"))
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
		docker := protected.Group("/docker", middleware.RequirePermission("docker"))
		{
			docker.GET("/summary", r.dockerHandler.GetSummary)

			docker.GET("/containers", r.dockerHandler.ListContainers)
			docker.GET("/containers/:id", r.dockerHandler.GetContainer)
			docker.POST("/containers/:id/start", r.dockerHandler.StartContainer)
			docker.POST("/containers/:id/stop", r.dockerHandler.StopContainer)
			docker.POST("/containers/:id/restart", r.dockerHandler.RestartContainer)
			docker.DELETE("/containers/:id", r.dockerHandler.DeleteContainer)

			docker.GET("/images", r.dockerHandler.ListImages)
			docker.POST("/images/pull", r.dockerHandler.PullImage)
			docker.DELETE("/images/:id", r.dockerHandler.DeleteImage)

			docker.GET("/networks", r.dockerHandler.ListNetworks)
			docker.POST("/networks", r.dockerHandler.CreateNetwork)
			docker.DELETE("/networks/:id", r.dockerHandler.DeleteNetwork)

			docker.GET("/volumes", r.dockerHandler.ListVolumes)
			docker.POST("/volumes", r.dockerHandler.CreateVolume)
			docker.DELETE("/volumes/:id", r.dockerHandler.DeleteVolume)

			docker.GET("/compose", r.dockerHandler.ListComposeStacks)
			docker.POST("/compose/deploy", r.dockerHandler.DeployCompose)
			docker.POST("/compose/stop", r.dockerHandler.StopCompose)
		}

		// Security
		security := protected.Group("/security", middleware.RequirePermission("settings"))
		{
			security.POST("/scans", r.securityHandler.CreateScan)
			security.GET("/scans", r.securityHandler.ListScans)
			security.GET("/scans/:id", r.securityHandler.GetScan)
			security.DELETE("/scans/:id", r.securityHandler.DeleteScan)

			security.GET("/scans/:id/vulnerabilities", r.securityHandler.ListVulnerabilitiesByScan)
			security.GET("/vulnerabilities", r.securityHandler.ListVulnerabilitiesByTenant)
			security.GET("/vulnerabilities/:id", r.securityHandler.GetVulnerability)
			security.PUT("/vulnerabilities/:id", r.securityHandler.UpdateVulnerability)
			security.DELETE("/vulnerabilities/:id", r.securityHandler.DeleteVulnerability)

			security.GET("/scans/:id/checks", r.securityHandler.ListChecksByScan)

			security.POST("/policies", r.securityHandler.CreatePolicy)
			security.GET("/policies", r.securityHandler.ListPolicies)
			security.GET("/policies/:id", r.securityHandler.GetPolicy)
			security.PUT("/policies/:id", r.securityHandler.UpdatePolicy)
			security.DELETE("/policies/:id", r.securityHandler.DeletePolicy)
		}

		// Files
		files := protected.Group("/files", middleware.RequirePermission("website"))
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
		cron := protected.Group("/cron", middleware.RequireExactPermission("terminal", "execute"))
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
		firewall := protected.Group("/firewall", middleware.RequireAdmin())
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
		backups := protected.Group("/backups", middleware.RequirePermission("backup"))
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
		services := protected.Group("/services", middleware.RequireAdmin())
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
		php := protected.Group("/php", middleware.RequirePermission("php"))
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
			php.GET("/versions/:id/extensions", r.phpHandler.ListPHPExtensions)
			php.PUT("/extensions/:id", r.phpHandler.UpdatePHPExtension)
			php.DELETE("/extensions/:id", r.phpHandler.DeletePHPExtension)

			php.GET("/versions/:id/config", r.phpHandler.GetPHPConfig)
			php.PUT("/versions/:id/config", r.phpHandler.UpdatePHPConfig)
			php.DELETE("/config/:id", r.phpHandler.DeletePHPConfig)
		}

		// Monitoring
		monitoring := protected.Group("/monitoring", middleware.RequirePermission("monitoring"))
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
		logs := protected.Group("/logs", middleware.RequirePermission("logs"))
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
		nodeApps := protected.Group("/node-apps", middleware.RequirePermission("nodeapp"))
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
		reverseProxy := protected.Group("/reverse-proxy", middleware.RequirePermission("reverseproxy"))
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
		gitDeployments := protected.Group("/git-deployments", middleware.RequireExactPermission("terminal", "execute"))
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
		wordpress := protected.Group("/wordpress", middleware.RequirePermission("wordpress"))
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
		notifications := protected.Group("/notifications", middleware.RequirePermission("notifications"))
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
		audit := protected.Group("/audit", middleware.RequirePermission("audit"))
		{
			audit.GET("/search", r.auditHandler.Search)
			audit.GET("/stats", r.auditHandler.GetStats)
			audit.GET("/:id", r.auditHandler.Get)
			audit.POST("/cleanup", r.auditHandler.CleanupOld)
		}

		// Clusters
		clusters := protected.Group("/clusters", middleware.RequireAdmin())
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
		loadBalancers := protected.Group("/load-balancers", middleware.RequireAdmin())
		{
			loadBalancers.POST("", r.clusterHandler.CreateLoadBalancer)
			loadBalancers.GET("", r.clusterHandler.ListLoadBalancers)
			loadBalancers.GET("/:id", r.clusterHandler.GetLoadBalancer)
			loadBalancers.PUT("/:id", r.clusterHandler.UpdateLoadBalancer)
			loadBalancers.DELETE("/:id", r.clusterHandler.DeleteLoadBalancer)
		}

		// HA Pairs
		haPairs := protected.Group("/ha-pairs", middleware.RequireAdmin())
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
		jobs := protected.Group("/jobs", middleware.RequirePermission("backup"))
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

		// API Keys
		apiKeys := protected.Group("/api-keys", middleware.RequirePermission("user"))
		{
			apiKeys.POST("", r.apiKeyHandler.Create)
			apiKeys.GET("", r.apiKeyHandler.List)
			apiKeys.GET("/:id", r.apiKeyHandler.Get)
			apiKeys.PUT("/:id", r.apiKeyHandler.Update)
			apiKeys.DELETE("/:id", r.apiKeyHandler.Delete)
		}

		// WAF (Web Application Firewall)
		waf := protected.Group("/waf", middleware.RequirePermission("settings"))
		{
			// Rules
			waf.GET("/rules", r.wafHandler.ListRules)
			waf.GET("/rules/:id", r.wafHandler.GetRule)
			waf.POST("/rules", r.wafHandler.CreateRule)
			waf.PUT("/rules/:id", r.wafHandler.UpdateRule)
			waf.DELETE("/rules/:id", r.wafHandler.DeleteRule)
			waf.POST("/rules/:id/toggle", r.wafHandler.ToggleRule)

			// Policies
			waf.GET("/policies", r.wafHandler.ListPolicies)
			waf.GET("/policies/:id", r.wafHandler.GetPolicy)
			waf.POST("/policies", r.wafHandler.CreatePolicy)
			waf.PUT("/policies/:id", r.wafHandler.UpdatePolicy)
			waf.DELETE("/policies/:id", r.wafHandler.DeletePolicy)

			// Events
			waf.GET("/events", r.wafHandler.ListEvents)

			// Stats
			waf.GET("/stats", r.wafHandler.GetStats)
		}

		// Website Statistics Pro
		websiteStats := protected.Group("/website-stats", middleware.RequirePermission("website"))
		{
			websiteStats.GET("/overview", r.websiteStatsHandler.GetOverview)
			websiteStats.GET("/visitors", r.websiteStatsHandler.ListVisitorLogs)
			websiteStats.POST("/visitors", r.websiteStatsHandler.RecordVisitorLog)
		}

		// Email Marketing
		emailMarketing := protected.Group("/email-marketing", middleware.RequirePermission("settings"))
		{
			emailMarketing.GET("/stats", r.emailMarketingHandler.GetStats)

			// Campaigns
			emailMarketing.GET("/campaigns", r.emailMarketingHandler.ListCampaigns)
			emailMarketing.POST("/campaigns", r.emailMarketingHandler.CreateCampaign)
			emailMarketing.GET("/campaigns/:id", r.emailMarketingHandler.GetCampaign)
			emailMarketing.PUT("/campaigns/:id", r.emailMarketingHandler.UpdateCampaign)
			emailMarketing.DELETE("/campaigns/:id", r.emailMarketingHandler.DeleteCampaign)
			emailMarketing.POST("/campaigns/:id/send", r.emailMarketingHandler.SendCampaign)
			emailMarketing.POST("/campaigns/:id/pause", r.emailMarketingHandler.PauseCampaign)

			// Contacts
			emailMarketing.GET("/contacts", r.emailMarketingHandler.ListContacts)
			emailMarketing.POST("/contacts", r.emailMarketingHandler.CreateContact)
			emailMarketing.DELETE("/contacts/:id", r.emailMarketingHandler.DeleteContact)

			// Lists
			emailMarketing.GET("/lists", r.emailMarketingHandler.ListLists)
			emailMarketing.POST("/lists", r.emailMarketingHandler.CreateList)
			emailMarketing.DELETE("/lists/:id", r.emailMarketingHandler.DeleteList)

			// Templates
			emailMarketing.GET("/templates", r.emailMarketingHandler.ListTemplates)
			emailMarketing.POST("/templates", r.emailMarketingHandler.CreateTemplate)
			emailMarketing.DELETE("/templates/:id", r.emailMarketingHandler.DeleteTemplate)
		}

		// File Protection
		fileProtection := protected.Group("/file-protection", middleware.RequirePermission("settings"))
		{
			fileProtection.GET("/stats", r.fileProtectionHandler.GetStats)

			// Rules
			fileProtection.GET("/rules", r.fileProtectionHandler.ListRules)
			fileProtection.POST("/rules", r.fileProtectionHandler.CreateRule)
			fileProtection.GET("/rules/:id", r.fileProtectionHandler.GetRule)
			fileProtection.PUT("/rules/:id", r.fileProtectionHandler.UpdateRule)
			fileProtection.DELETE("/rules/:id", r.fileProtectionHandler.DeleteRule)
			fileProtection.POST("/rules/:id/toggle", r.fileProtectionHandler.ToggleRule)

			// Events
			fileProtection.GET("/events", r.fileProtectionHandler.ListEvents)
			fileProtection.PUT("/events/:id/read", r.fileProtectionHandler.MarkEventRead)
			fileProtection.PUT("/events/read-all", r.fileProtectionHandler.MarkAllEventsRead)

			// Quarantine
			fileProtection.GET("/quarantine", r.fileProtectionHandler.ListQuarantine)
			fileProtection.POST("/quarantine/:id/restore", r.fileProtectionHandler.RestoreQuarantine)
			fileProtection.DELETE("/quarantine/:id", r.fileProtectionHandler.DeleteQuarantine)
		}

		// Mail Server
		mailServer := protected.Group("/mail-server", middleware.RequirePermission("settings"))
		{
			mailServer.GET("/stats", r.mailServerHandler.GetStats)

			// Domains
			mailServer.GET("/domains", r.mailServerHandler.ListDomains)
			mailServer.POST("/domains", r.mailServerHandler.CreateDomain)
			mailServer.GET("/domains/:id", r.mailServerHandler.GetDomain)
			mailServer.DELETE("/domains/:id", r.mailServerHandler.DeleteDomain)

			// Accounts
			mailServer.GET("/accounts", r.mailServerHandler.ListAccounts)
			mailServer.POST("/accounts", r.mailServerHandler.CreateAccount)
			mailServer.GET("/accounts/:id", r.mailServerHandler.GetAccount)
			mailServer.PUT("/accounts/:id", r.mailServerHandler.UpdateAccount)
			mailServer.DELETE("/accounts/:id", r.mailServerHandler.DeleteAccount)

			// Aliases
			mailServer.GET("/aliases", r.mailServerHandler.ListAliases)
			mailServer.POST("/aliases", r.mailServerHandler.CreateAlias)
			mailServer.DELETE("/aliases/:id", r.mailServerHandler.DeleteAlias)

			// Queue
			mailServer.GET("/queue", r.mailServerHandler.ListQueue)
			mailServer.DELETE("/queue/:id", r.mailServerHandler.DeleteQueueItem)
			mailServer.POST("/queue/flush", r.mailServerHandler.FlushQueue)

			// Spam Filter
			mailServer.GET("/spam-filter", r.mailServerHandler.GetSpamFilter)
			mailServer.PUT("/spam-filter", r.mailServerHandler.UpdateSpamFilter)

			// Server Config
			mailServer.GET("/config", r.mailServerHandler.GetServerConfig)
			mailServer.PUT("/config", r.mailServerHandler.UpdateServerConfig)
		}

		// Multi-user Management
		multiUser := protected.Group("/multi-user", middleware.RequireAdmin())
		{
			multiUser.GET("/stats", r.multiUserHandler.GetStats)

			// Roles
			multiUser.GET("/roles", r.multiUserHandler.ListRoles)
			multiUser.POST("/roles", r.multiUserHandler.CreateRole)
			multiUser.GET("/roles/:id", r.multiUserHandler.GetRole)
			multiUser.PUT("/roles/:id", r.multiUserHandler.UpdateRole)
			multiUser.DELETE("/roles/:id", r.multiUserHandler.DeleteRole)

			// Permissions
			multiUser.GET("/permissions", r.multiUserHandler.ListPermissions)

			// User-Role assignment
			multiUser.POST("/users/:id/roles", r.multiUserHandler.AssignUserRole)
			multiUser.DELETE("/users/:id/roles/:roleId", r.multiUserHandler.RemoveUserRole)
			multiUser.GET("/users/:id/roles", r.multiUserHandler.GetUserRoles)
			multiUser.GET("/users/:id/permissions", r.multiUserHandler.GetUserPermissions)

			// Sessions
			multiUser.GET("/sessions", r.multiUserHandler.ListActiveSessions)
			multiUser.DELETE("/sessions/:id", r.multiUserHandler.DeleteSession)
			multiUser.DELETE("/users/:id/sessions", r.multiUserHandler.TerminateUserSessions)

			// Activity log
			multiUser.GET("/activities", r.multiUserHandler.ListActivities)
		}

		// Scheduled Tasks Pro
		scheduledTasks := protected.Group("/scheduled-tasks", middleware.RequireExactPermission("terminal", "execute"))
		{
			scheduledTasks.GET("/stats", r.scheduledTaskHandler.GetStats)

			// Tasks
			scheduledTasks.GET("", r.scheduledTaskHandler.ListTasks)
			scheduledTasks.POST("", r.scheduledTaskHandler.CreateTask)
			scheduledTasks.GET("/recent-executions", r.scheduledTaskHandler.ListRecentExecutions)
			scheduledTasks.GET("/:id", r.scheduledTaskHandler.GetTask)
			scheduledTasks.PUT("/:id", r.scheduledTaskHandler.UpdateTask)
			scheduledTasks.DELETE("/:id", r.scheduledTaskHandler.DeleteTask)
			scheduledTasks.POST("/:id/toggle", r.scheduledTaskHandler.ToggleTask)
			scheduledTasks.POST("/:id/run", r.scheduledTaskHandler.RunTask)
			scheduledTasks.GET("/:id/executions", r.scheduledTaskHandler.ListExecutions)

			// Executions
			scheduledTasks.GET("/executions/:id", r.scheduledTaskHandler.GetExecution)
			scheduledTasks.POST("/executions/cleanup", r.scheduledTaskHandler.CleanupExecutions)

			// Templates
			scheduledTasks.GET("/templates", r.scheduledTaskHandler.ListTemplates)
			scheduledTasks.POST("/templates", r.scheduledTaskHandler.CreateTemplate)
			scheduledTasks.DELETE("/templates/:id", r.scheduledTaskHandler.DeleteTemplate)

			// Groups
			scheduledTasks.GET("/groups", r.scheduledTaskHandler.ListGroups)
			scheduledTasks.POST("/groups", r.scheduledTaskHandler.CreateGroup)
			scheduledTasks.DELETE("/groups/:id", r.scheduledTaskHandler.DeleteGroup)
		}

		// Tamper Proof for Enterprise Pro
		tamperProof := protected.Group("/tamper-proof", middleware.RequirePermission("settings"))
		{
			tamperProof.GET("/stats", r.tamperProofHandler.GetStats)

			// Protected Paths
			tamperProof.GET("/paths", r.tamperProofHandler.ListProtectedPaths)
			tamperProof.POST("/paths", r.tamperProofHandler.CreateProtectedPath)
			tamperProof.GET("/paths/:id", r.tamperProofHandler.GetProtectedPath)
			tamperProof.PUT("/paths/:id", r.tamperProofHandler.UpdateProtectedPath)
			tamperProof.DELETE("/paths/:id", r.tamperProofHandler.DeleteProtectedPath)

			// Scanning
			tamperProof.POST("/paths/:id/scan", r.tamperProofHandler.Scan)
			tamperProof.POST("/scan-all", r.tamperProofHandler.ScanAll)

			// Baselines
			tamperProof.GET("/paths/:id/baselines", r.tamperProofHandler.GetBaselines)
			tamperProof.POST("/paths/:id/baselines/refresh", r.tamperProofHandler.RefreshBaseline)

			// Alerts
			tamperProof.GET("/alerts", r.tamperProofHandler.ListAlerts)
			tamperProof.GET("/alerts/:id", r.tamperProofHandler.GetAlert)
			tamperProof.POST("/alerts/:id/resolve", r.tamperProofHandler.ResolveAlert)

			// Scan Results
			tamperProof.GET("/scan-results", r.tamperProofHandler.ListScanResults)

			// Audit Logs
			tamperProof.GET("/audit-logs", r.tamperProofHandler.ListAuditLogs)

			// Cleanup
			tamperProof.POST("/cleanup", r.tamperProofHandler.Cleanup)
		}

		// Daily Reports
		dailyReports := protected.Group("/daily-reports", middleware.RequirePermission("monitoring"))
		{
			dailyReports.GET("/stats", r.dailyReportHandler.GetStats)

			// Reports
			dailyReports.GET("/reports", r.dailyReportHandler.ListReports)
			dailyReports.POST("/reports/generate", r.dailyReportHandler.GenerateReport)
			dailyReports.GET("/reports/:id", r.dailyReportHandler.GetReport)
			dailyReports.DELETE("/reports/:id", r.dailyReportHandler.DeleteReport)

			// Schedules
			dailyReports.GET("/schedules", r.dailyReportHandler.ListSchedules)
			dailyReports.POST("/schedules", r.dailyReportHandler.CreateSchedule)
			dailyReports.GET("/schedules/:id", r.dailyReportHandler.GetSchedule)
			dailyReports.PUT("/schedules/:id", r.dailyReportHandler.UpdateSchedule)
			dailyReports.DELETE("/schedules/:id", r.dailyReportHandler.DeleteSchedule)

			// Deliveries
			dailyReports.GET("/deliveries", r.dailyReportHandler.ListDeliveries)
		}

		// Panel access settings. Administrator only: these routes change the
		// port, the security entrance, the IP allow list and the panel's own
		// TLS, which is to say they change who can reach the panel at all.
		panel := protected.Group("/panel", middleware.RequireAdmin())
		{
			panel.GET("/settings", r.panelSettingsHandler.Get)
			panel.PUT("/settings", r.panelSettingsHandler.Update)
			panel.POST("/settings/entrance/regenerate", r.panelSettingsHandler.RegenerateEntrance)
			panel.POST("/settings/tls/reissue", r.panelSettingsHandler.ReissueCertificate)
		}

		// Config
		configRoutes := protected.Group("/config", middleware.RequireAdmin())
		{
			configRoutes.POST("/snapshots", r.configHandler.CreateSnapshot)
			configRoutes.GET("/snapshots", r.configHandler.ListSnapshots)
			configRoutes.GET("/snapshots/:id", r.configHandler.GetSnapshot)
			configRoutes.DELETE("/snapshots/:id", r.configHandler.DeleteSnapshot)
			configRoutes.POST("/rollback", r.configHandler.Rollback)
			configRoutes.GET("/diff", r.configHandler.GetDiff)
			configRoutes.GET("/history", r.configHandler.GetSnapshotHistory)
			configRoutes.GET("/stats", r.configHandler.GetConfigStats)
			configRoutes.POST("/cleanup", r.configHandler.CleanupOldSnapshots)
			configRoutes.POST("/validate", r.configHandler.ValidateConfig)

			configRoutes.POST("/templates", r.configHandler.CreateTemplate)
			configRoutes.GET("/templates", r.configHandler.ListTemplates)
			configRoutes.GET("/templates/:id", r.configHandler.GetTemplate)
			configRoutes.PUT("/templates/:id", r.configHandler.UpdateTemplate)
			configRoutes.DELETE("/templates/:id", r.configHandler.DeleteTemplate)
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
