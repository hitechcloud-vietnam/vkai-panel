package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/database"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/handler"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/tlsmanager"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/websocket"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg.Log, cfg.Paths.LogRoot)
	defer logger.Sync()

	// Panel access gate. The panel gets its own port and its own secret
	// entrance: 80/443 belong to the websites this server hosts, and an admin
	// API answering there is reachable by every scanner on the internet.
	panelCfg, err := config.LoadPanelAccess()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load panel access config: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Starting VKAI Panel API",
		zap.Int("panel_port", panelCfg.Port),
		zap.String("panel_bind", panelCfg.Bind),
		zap.Bool("entrance_enabled", panelCfg.EntranceEnabled),
		zap.Bool("tls", panelCfg.TLS.Enabled),
		zap.String("tls_mode", panelCfg.TLSMode()),
	)

	// Connect to database
	db, err := database.Connect(cfg.Database, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Connect to Redis
	rdb, err := database.ConnectRedis(cfg.Redis, logger)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer rdb.Close()

	// Initialize JWT manager
	jwtManager := auth.NewJWTManager(
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
		cfg.JWT.Issuer,
	)

	// Initialize repositories
	tenantRepo := repository.NewTenantRepository(db.DB)
	userRepo := repository.NewUserRepository(db.DB)
	serverRepo := repository.NewServerRepository(db.DB)
	websiteRepo := repository.NewWebsiteRepository(db.DB)
	sslRepo := repository.NewSSLRepository(db.DB)
	dbRepo := repository.NewDatabaseRepository(db.DB)
	cronRepo := repository.NewCronRepository(db.DB)
	firewallRepo := repository.NewFirewallRepository(db.DB)
	backupRepo := repository.NewBackupRepository(db.DB)
	phpRepo := repository.NewPHPRepository(db.DB)
	dnsRepo := repository.NewDNSRepository(db.DB)
	securityRepo := repository.NewSecurityRepository(db.DB)
	nodeAppRepo := repository.NewNodeAppRepository(db.DB)

	// Initialize webserver registry
	webserverRegistry := webserver.NewRegistry()

	// Initialize services
	tenantService := service.NewTenantService(tenantRepo, logger)
	userService := service.NewUserService(userRepo, tenantRepo, logger)
	authService := service.NewAuthService(userRepo, tenantRepo, jwtManager, logger)
	serverService := service.NewServerService(serverRepo, logger)
	websiteService := service.NewWebsiteService(websiteRepo, serverRepo, webserverRegistry)
	sslService := service.NewSSLService(sslRepo, websiteRepo)
	dbService := service.NewDatabaseService(dbRepo, serverRepo)
	cronService := service.NewCronService(cronRepo, serverRepo)
	firewallService := service.NewFirewallService(firewallRepo, serverRepo)
	backupService := service.NewBackupService(backupRepo)
	serviceManager := service.NewServiceManager()
	fileManager := service.NewFileManager("/")
	phpService := service.NewPHPService(phpRepo, logger)
	dnsService := service.NewDNSService(dnsRepo, logger)
	securityService := service.NewSecurityService(securityRepo, logger)
	nodeAppService := service.NewNodeAppService(nodeAppRepo, logger)

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(logger)
	authHandler := handler.NewAuthHandler(authService, logger)
	tenantHandler := handler.NewTenantHandler(tenantService, logger)
	userHandler := handler.NewUserHandler(userService, logger)
	serverHandler := handler.NewServerHandler(serverService, logger)
	websiteHandler := handler.NewWebsiteHandler(websiteService)
	sslHandler := handler.NewSSLHandler(sslService)
	databaseHandler := handler.NewDatabaseHandler(dbService)
	cronHandler := handler.NewCronHandler(cronService)
	firewallHandler := handler.NewFirewallHandler(firewallService)
	backupHandler := handler.NewBackupHandler(backupService)
	serviceHandler := handler.NewServiceHandler(serviceManager)
	fileManagerHandler := handler.NewFileManagerHandler(fileManager)

	// Initialize monitoring
	monitoringRepo := repository.NewMonitoringRepository(db.DB)
	monitoringService := service.NewMonitoringService(monitoringRepo, logger)
	monitoringHandler := handler.NewMonitoringHandler(monitoringService, logger)

	phpHandler := handler.NewPHPHandler(phpService, logger)
	dnsHandler := handler.NewDNSHandler(dnsService, logger)
	securityHandler := handler.NewSecurityHandler(securityService, logger)
	nodeAppHandler := handler.NewNodeAppHandler(nodeAppService, logger)

	// Initialize reverse proxy
	reverseProxyRepo := repository.NewReverseProxyRepository(db.DB)
	reverseProxyService := service.NewReverseProxyService(reverseProxyRepo, logger)
	reverseProxyHandler := handler.NewReverseProxyHandler(reverseProxyService)

	// Initialize git deployment
	gitDeploymentRepo := repository.NewGitDeploymentRepository(db.DB)
	gitDeploymentService := service.NewGitDeploymentService(gitDeploymentRepo, logger)
	gitDeploymentHandler := handler.NewGitDeploymentHandler(gitDeploymentService)

	// Initialize WordPress
	wordpressRepo := repository.NewWordPressRepository(db.DB)
	wordpressService := service.NewWordPressService(wordpressRepo, logger)
	wordpressHandler := handler.NewWordPressHandler(wordpressService)

	// Initialize log management
	logRepo := repository.NewLogRepository(db.DB)
	logService := service.NewLogService(logRepo, logger)
	logHandler := handler.NewLogHandler(logService, logger)

	// Initialize notifications
	notificationRepo := repository.NewNotificationRepository(db.DB)
	notificationService := service.NewNotificationService(notificationRepo, logger)
	notificationHandler := handler.NewNotificationHandler(notificationService, logger)

	// Initialize audit logging
	auditRepo := repository.NewAuditRepository(db.DB)
	auditService := service.NewAuditService(auditRepo, logger)
	auditHandler := handler.NewAuditHandler(auditService, logger)

	// Initialize cluster management
	clusterRepo := repository.NewClusterRepository(db.DB)
	clusterService := service.NewClusterService(clusterRepo, logger)
	clusterHandler := handler.NewClusterHandler(clusterService, logger)

	// Initialize WebSocket hub
	wsHub := websocket.NewHub(logger)
	go wsHub.Run()

	// Initialize WebSocket handler
	wsHandler := handler.NewWebSocketHandler(wsHub, logger)

	// Initialize job queue
	jobQueue := job.NewQueueManager("localhost:6379", "", 0, logger)
	defer jobQueue.Close()

	// Initialize job service
	jobRepo := repository.NewJobRepository(db.DB)
	jobService := service.NewJobService(jobRepo, jobQueue, logger)
	jobHandler := handler.NewJobHandler(jobService, logger)

	// Start job worker in background
	go func() {
		jobQueue.StartWorker(10)
	}()

	// Initialize config management
	configRepo := repository.NewConfigRepository(db.DB)
	configService := service.NewConfigService(configRepo, logger)
	configHandler := handler.NewConfigHandler(configService, logger)

	// Initialize Docker management
	dockerHandler := handler.NewDockerHandler(logger)

	// Initialize API keys
	apiKeyRepo := repository.NewAPIKeyRepository(db.DB)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, logger)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService, logger)

	// Initialize WAF (Web Application Firewall)
	wafRepo := repository.NewWAFRepository(db.DB)
	wafService := service.NewWAFService(wafRepo)
	wafHandler := handler.NewWAFHandler(wafService, logger)

	// Initialize Website Statistics Pro
	websiteStatsRepo := repository.NewWebsiteStatsRepository(db.DB)
	websiteStatsService := service.NewWebsiteStatsService(websiteStatsRepo, logger)
	websiteStatsHandler := handler.NewWebsiteStatsHandler(websiteStatsService, logger)

	// Initialize Email Marketing
	emailMarketingRepo := repository.NewEmailMarketingRepository(db.DB, logger)
	emailMarketingService := service.NewEmailMarketingService(emailMarketingRepo, logger)
	emailMarketingHandler := handler.NewEmailMarketingHandler(emailMarketingService, logger)

	// Initialize Mail Server
	mailServerRepo := repository.NewMailServerRepository(db.DB, logger)
	mailServerService := service.NewMailServerService(mailServerRepo, logger)
	mailServerHandler := handler.NewMailServerHandler(mailServerService, logger)

	// Initialize File Protection
	fileProtectionRepo := repository.NewFileProtectionRepository(db.DB, logger)
	fileProtectionService := service.NewFileProtectionService(fileProtectionRepo, logger)
	fileProtectionHandler := handler.NewFileProtectionHandler(fileProtectionService, logger)

	// Initialize Multi-user Management
	multiUserRepo := repository.NewMultiUserRepository(db.DB, logger)
	multiUserService := service.NewMultiUserService(multiUserRepo, logger)
	multiUserHandler := handler.NewMultiUserHandler(multiUserService, logger)

	// Initialize Daily Report Pro
	dailyReportRepo := repository.NewDailyReportRepository(db.DB, logger)
	dailyReportService := service.NewDailyReportService(dailyReportRepo, logger)
	dailyReportHandler := handler.NewDailyReportHandler(dailyReportService, logger)

	// Initialize Scheduled Tasks Pro
	scheduledTaskRepo := repository.NewScheduledTaskRepository(db.DB, logger)
	scheduledTaskService := service.NewScheduledTaskService(scheduledTaskRepo, logger)
	scheduledTaskHandler := handler.NewScheduledTaskHandler(scheduledTaskService, logger)

	// Initialize Tamper Proof for Enterprise Pro
	tamperProofRepo := repository.NewTamperProofRepository(db.DB, logger)
	tamperProofService := service.NewTamperProofService(tamperProofRepo, logger)
	tamperProofHandler := handler.NewTamperProofHandler(tamperProofService, logger)

	// Setup router
	router := handler.NewRouter(
		authHandler,
		serverHandler,
		tenantHandler,
		userHandler,
		healthHandler,
		websiteHandler,
		sslHandler,
		databaseHandler,
		cronHandler,
		firewallHandler,
		backupHandler,
		serviceHandler,
		fileManagerHandler,
		monitoringHandler,
		logHandler,
		notificationHandler,
		auditHandler,
		clusterHandler,
		phpHandler,
		dnsHandler,
		securityHandler,
		nodeAppHandler,
		reverseProxyHandler,
		gitDeploymentHandler,
		wordpressHandler,
		wsHandler,
		jobHandler,
		configHandler,
		dockerHandler,
		apiKeyHandler,
		wafHandler,
		websiteStatsHandler,
		emailMarketingHandler,
		mailServerHandler,
		fileProtectionHandler,
		multiUserHandler,
		dailyReportHandler,
		scheduledTaskHandler,
		tamperProofHandler,
		jwtManager,
		logger,
	)

	engine := router.Setup()

	// The access gate wraps the whole engine rather than being registered as a
	// route middleware, so it also covers routes registered before it and
	// anything mounted later: there is no ordering in which a request can reach
	// a handler without passing the gate first.
	panelGuard, err := middleware.NewPanelGuardFromConfig(panelCfg, cfg.JWT.Secret, logger)
	if err != nil {
		logger.Fatal("Failed to build panel access gate", zap.Error(err))
	}

	// Panel TLS is optional and independent of the customer vhosts' certificates.
	//
	// The manager owns the certificate for the whole life of the process: it
	// serves it through tls.Config.GetCertificate, so a renewal is a pointer
	// swap rather than a restart. That is what makes Let's Encrypt usable here
	// at all - a certificate issued for a bare IP address comes from the
	// "shortlived" profile and expires in about six days.
	tlsCtx, cancelTLS := context.WithCancel(context.Background())
	defer cancelTLS()

	tlsManager, err := tlsmanager.New(tlsmanager.Options{
		Config: panelCfg,
		Logger: logger,
	})
	if err != nil {
		logger.Fatal("Failed to prepare panel TLS", zap.Error(err))
	}
	if err := tlsManager.Start(tlsCtx); err != nil {
		logger.Fatal("Failed to prepare panel TLS", zap.Error(err))
	}
	defer tlsManager.Stop()

	if panelCfg.TLS.Enabled {
		info := tlsManager.Info()
		logger.Info("Panel TLS ready",
			zap.String("mode", info.Mode),
			zap.String("certificate_source", info.Source),
			zap.String("identifier_type", info.IdentifierType),
			zap.String("identifier", info.Identifier),
			zap.String("profile", info.Profile),
			zap.Bool("acme_staging", info.Staging),
			zap.Time("not_after", info.NotAfter),
			zap.String("last_error", info.LastError),
		)
	}

	// The listen address comes from the panel access config; the legacy
	// server.host/server.port pair is only used when the gate is switched off.
	addr := panelCfg.ListenAddr()
	if !panelCfg.Enabled {
		addr = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         addr,
		Handler:      panelGuard.Wrap(engine),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}
	if panelCfg.TLS.Enabled {
		srv.TLSConfig = tlsManager.TLSConfig()
	}

	// Print the access table once, the way aaPanel does, so an operator can copy
	// the URL out of the console before closing the session.
	if panelCfg.Enabled {
		fmt.Print(panelCfg.Banner())
	}

	// Start server in goroutine
	go func() {
		logger.Info("Panel server starting",
			zap.String("addr", srv.Addr),
			zap.String("scheme", panelCfg.Scheme()),
			zap.String("certificate_source", tlsManager.Source()),
		)

		var err error
		if panelCfg.TLS.Enabled {
			// Empty file names: the certificate comes from srv.TLSConfig's
			// GetCertificate, so a renewal never has to reload a path.
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited properly")
}

func initLogger(cfg config.LogConfig, logRoot string) *zap.Logger {
	if strings.TrimSpace(logRoot) == "" {
		logRoot = config.LogRoot()
	}
	level := zap.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zap.DebugLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	}

	// File rotation
	fileWriter := &lumberjack.Logger{
		Filename:   filepath.Join(logRoot, "api.log"),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}

	// Console + file core
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(fileEncoder, zapcore.AddSync(fileWriter), level),
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}
