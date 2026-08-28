package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/database"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/handler"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/job"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/reload"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/uiproxy"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/version"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/webserver"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/websocket"
)

func main() {
	// "vkai-api --version" answers before anything else is touched: no
	// configuration, no database, no port. It is how an operator - or a support
	// ticket - finds out which release is actually installed, and how a build
	// can be checked for the linker stamp it is supposed to carry.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-version", "-v", "version":
			fmt.Printf("VKAI Panel API %s\n", version.String())
			return
		}
	}

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

	// A configuration that names a certificate mode and disables TLS in the same
	// breath leaves the whole certificate machinery constructed and idle. That
	// shipped once and produced no diagnostic at all, which is why this is loud.
	if msg := panelCfg.TLSInconsistency(); msg != "" {
		logger.Warn("Panel TLS configuration is self-contradictory", zap.String("detail", msg))
	}

	// The reload supervisor is built here, before anything else that reads the
	// panel configuration, and published immediately.
	//
	// It has to be published this early because the router constructs the panel
	// settings service itself, positionally, out of reach of this function - so
	// a process-wide handoff is the only way that service can be handed the
	// running panel instead of a second, disconnected copy of its
	// configuration. The appliers that make a change real are registered
	// further down, once the handler, the certificate manager and the listener
	// exist.
	supervisor := reload.New(reload.Options{Config: panelCfg, Logger: logger})
	reload.SetDefault(supervisor)

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

	// HOSTING PACKAGES AND QUOTA.
	//
	// The enforcer is built BEFORE the services, because every service that can
	// create a limited resource takes it as a required constructor argument.
	// That is deliberate: a quota check that can be forgotten is a quota check
	// that will be, and this way forgetting it does not compile.
	//
	// A nil store here would not disable enforcement either - Check refuses
	// rather than allows when it has nothing to ask.
	quotaEnforcer := quota.New(quota.NewPostgresStore(db.DB), logger)

	// Initialize services
	tenantService := service.NewTenantService(tenantRepo, logger)
	userService := service.NewUserService(userRepo, tenantRepo, logger)
	authService := service.NewAuthService(userRepo, tenantRepo, jwtManager, logger)
	serverService := service.NewServerService(serverRepo, logger)
	websiteService := service.NewWebsiteService(websiteRepo, serverRepo, webserverRegistry, quotaEnforcer)
	sslService := service.NewSSLService(sslRepo, websiteRepo)
	dbService := service.NewDatabaseService(dbRepo, serverRepo, quotaEnforcer)
	cronService := service.NewCronService(cronRepo, serverRepo, quotaEnforcer)
	firewallService := service.NewFirewallService(firewallRepo, serverRepo)
	backupService := service.NewBackupService(backupRepo)
	serviceManager := service.NewServiceManager()
	fileManager := service.NewFileManager("/")
	phpService := service.NewPHPService(phpRepo, logger)
	dnsService := service.NewDNSService(dnsRepo, logger)
	securityService := service.NewSecurityService(securityRepo, logger)
	nodeAppService := service.NewNodeAppService(nodeAppRepo, logger)

	// Suspending an account has to take its websites offline, and the website
	// service is what can do that. The enforcer needs the website service and
	// the website service needs the enforcer, so this half of the loop is
	// closed with a setter rather than a constructor argument.
	//
	// Without it a suspension still refuses every new resource; it just does
	// not stop the vhosts, and the enforcer says so in the log when it happens.
	quotaEnforcer.SetSiteController(websiteService)

	// Disk and bandwidth are measured, not counted, and measuring them is
	// expensive: see internal/quota/measure.go for the cost of walking a large
	// tree and the budget that bounds it. The sampler runs on a schedule, one
	// account at a time, and never on a request.
	quotaSampler := quota.NewSampler(quota.SamplerOptions{
		Store:    quotaEnforcer.Store(),
		Enforcer: quotaEnforcer,
		Logger:   logger,
	})
	quotaCtx, cancelQuota := context.WithCancel(context.Background())
	defer cancelQuota()
	go quotaSampler.Run(quotaCtx)

	// The administrative surface: packages, assignment, overrides, suspension
	// and the usage report. It shares the one enforcer with the creation paths,
	// so what the panel displays and what the panel enforces cannot diverge.
	packageHandler := handler.NewPackageHandler(
		service.NewPackageService(quotaEnforcer, quotaSampler, logger), logger)
	handler.UsePackageHandler(packageHandler, logger)

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

	// The sign-in gate writes to the same trail. Without this line a granted
	// or refused sign-in is in the service log and nowhere an auditor can
	// verify, so SetAudit warns loudly when it is passed nothing.
	authService.SetAudit(auditService)

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

	// Panel sessions: the record that makes a stateless JWT revocable, and the
	// binding that stops a stolen token being usable from anywhere.
	//
	// The user repository is passed so that an operator whose address changed
	// can prove their password and carry on, rather than being signed out -
	// see internal/auth/sessionbinding.go for why the policy is shaped that
	// way.
	sessionRepo := repository.NewPanelSessionRepository(db.DB)
	sessionService := service.NewSessionService(sessionRepo, userRepo, auditService, logger)
	sessionHandler := handler.NewSessionHandler(sessionService, logger)

	// The router has no field for this handler and NewRouter is called
	// positionally, so it is installed the way the credential limiter is.
	// Without it the session routes still exist and answer 503 with the
	// reason.
	handler.SetSessionHandler(sessionHandler)

	// Initialize API keys
	apiKeyRepo := repository.NewAPIKeyRepository(db.DB)
	// The user repository is the authority loader: a key's scopes are bounded
	// by the roles and permissions of the account it belongs to, which have to
	// be read from somewhere that knows them.
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, userRepo, auditService, logger)
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
	mailServerService := service.NewMailServerService(mailServerRepo, logger, quotaEnforcer)
	mailServerHandler := handler.NewMailServerHandler(mailServerService, logger)

	// Initialize File Protection
	fileProtectionRepo := repository.NewFileProtectionRepository(db.DB, logger)
	fileProtectionService := service.NewFileProtectionService(fileProtectionRepo, logger)
	fileProtectionHandler := handler.NewFileProtectionHandler(fileProtectionService, logger)

	// Initialize Multi-user Management
	multiUserRepo := repository.NewMultiUserRepository(db.DB, logger)
	multiUserService := service.NewMultiUserService(multiUserRepo, logger)
	multiUserHandler := handler.NewMultiUserHandler(multiUserService, logger)
	// Role and permission changes go into the tamper-evident trail.
	multiUserHandler.SetAudit(auditService)

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

	// Two-factor authentication. One constructor builds the whole stack: the
	// Postgres store, the secret box keyed by VKAI_SECRET_KEY, and the
	// in-process attempt limiter. The audit service is passed so every
	// enrolment, regeneration and removal lands in the panel's trail.
	//
	// Without the master key a TOTP secret cannot be stored safely, so the
	// handler is not built. That is not fatal: the router serves the same
	// paths with a 503 that names the cause, which an operator can fix, rather
	// than 404s that read as "this panel has no two-factor".
	twoFactorHandler, err := handler.NewTwoFactorHandlerFromDB(db.DB, auditService, nil, "VKAI Panel", logger)
	if err != nil {
		logger.Error("Two-factor authentication is unavailable: enrolment and verification will refuse every request",
			zap.Error(err))
		twoFactorHandler = nil
	}

	// The agent channel's certificate authority, opened once here: the same
	// authority signs agent enrolments and holds the panel's own client
	// certificate. A CA that cannot be opened yields a handler that answers
	// 503 rather than a process that refuses to start - a panel that cannot
	// enrol agents is degraded, a panel that will not boot is an outage.
	agentPKIHandler := handler.NewAgentPKIHandlerFromEnv(jwtManager, logger)
	// Enrolment and revocation change which machines this panel takes orders
	// from, so both go into the trail.
	agentPKIHandler.SetAudit(auditService)

	// The credential limiter counts authentication failures across every
	// instance of the panel, so it is built on the Redis connection this
	// process already holds. Installed before the router is built, because the
	// guards ProtectCredentialEndpoints creates capture it once.
	middleware.SetCredentialLimiter(ratelimit.New(ratelimit.NewRedisStore(rdb.Client), ratelimit.PolicyFromEnv()))

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
		twoFactorHandler,
		agentPKIHandler,
		jwtManager,
		logger,
	)

	engine := router.Setup()

	// ========================================================================
	// HOSTING PACKAGE AND QUOTA ROUTES - THE LINE THAT MAKES THEM REACHABLE.
	//
	// Deleting the call below removes /api/v1/packages and /api/v1/quota from
	// the running panel. It is here rather than in internal/handler/router.go
	// only because that file is owned elsewhere; the group is built exactly the
	// way router.go builds its own `protected` group, so moving it later is a
	// one-line change. See the mounting comment in internal/handler/package.go.
	//
	// TestPackageRoutesAreMountedByMain in internal/handler fails if this is
	// removed.
	// ========================================================================
	quotaRoutes := engine.Group("/api/v1")
	quotaRoutes.Use(middleware.AuthRequired(jwtManager))
	handler.RegisterPackageRoutes(quotaRoutes)

	// ========================================================================
	// SCOPED API KEYS AND SESSION MANAGEMENT - THE LINE THAT MAKES THEM
	// REACHABLE.
	//
	// Deleting the call below removes /api/v1/integration (the scoped,
	// API-key-authenticated surface), API key rotation and revocation, and
	// every session endpoint from the running panel. The feature would still
	// compile and its tests would still pass, which is exactly how two-factor
	// authentication, the agent mTLS channel, the credential limiter and panel
	// TLS each came to be shipped unreachable.
	//
	// It is here rather than in internal/handler/router.go only because that
	// file is owned elsewhere. The call is idempotent, so the same line can be
	// added there - r.RegisterAccessRoutes() at the end of Setup() - and this
	// one then removed.
	//
	// TestAccessRoutesAreMountedByMain in internal/handler fails if this is
	// removed.
	// ========================================================================
	router.RegisterAccessRoutes()

	// The access gate wraps the whole engine rather than being registered as a
	// route middleware, so it also covers routes registered before it and
	// anything mounted later: there is no ordering in which a request can reach
	// a handler without passing the gate first.
	//
	// It is wrapped in a switch rather than built once. A gate is immutable by
	// construction - it compiles its entrance, its address matchers and its
	// cookie key at build time - so changing the entrance means building a new
	// gate and publishing it with one atomic store. That is what makes a
	// configuration change atomic from a request's point of view: a request
	// reads the pointer once and is then checked against one entrance, one
	// allow list and one pinned domain, all from the same configuration.

	// The panel has ONE front door, and it is this process. nginx forwards the
	// whole panel port here; requests the API does not own are forwarded to the
	// Next.js service afterwards. Serving the interface beside the gate instead
	// of behind it is what made the entrance decorative: the login form was
	// reachable by anyone who found the port, while only /api ever met the
	// guard.
	//
	// The proxy is deliberately outside the gin engine. The engine's response
	// headers are written for a JSON API - "Content-Security-Policy: default-src
	// 'none'" among them - and would break every page of the interface.
	// Session binding wraps the whole engine for the same reason the access
	// gate below does: a middleware attached to a group only guards the routes
	// somebody remembered to put on that group, while a wrapper guards
	// everything, including whatever is mounted next month.
	//
	// It acts only on requests carrying a valid access token. A request with
	// no token, or with an API key, passes straight through to the route that
	// would have handled it.
	apiHandler := middleware.BindSessions(engine, middleware.SessionGuardOptions{
		Evaluator: sessionService,
		JWT:       jwtManager,
		Logger:    logger,
	})

	panelHandler, err := uiproxy.New(uiproxy.Options{
		Upstream: cfg.UI.Upstream,
		API:      apiHandler,
		Logger:   logger,
	})
	if err != nil {
		logger.Fatal("Failed to build the panel UI proxy", zap.Error(err))
	}
	logger.Info("Panel UI upstream", zap.String("upstream", cfg.UI.Upstream))

	// Panel TLS is optional and independent of the customer vhosts' certificates.
	//
	// The manager owns the certificate for the whole life of the process: it
	// serves it through tls.Config.GetCertificate, so a renewal is a pointer
	// swap rather than a restart. That is what makes Let's Encrypt usable here
	// at all - a certificate issued for a bare IP address comes from the
	// "shortlived" profile and expires in about six days.
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	// The switch owns one certificate manager at a time. A mode change - the
	// operator moving from an automatically issued certificate to one they
	// pasted, or back - stops the old manager and starts a new one, which is
	// what keeps an ACME renewal from overwriting a pasted certificate weeks
	// later, and what makes the renewal loop resume when they change their mind.
	tlsSwitch, err := reload.NewTLSSwitch(runCtx, panelCfg, logger)
	if err != nil {
		logger.Fatal("Failed to prepare panel TLS", zap.Error(err))
	}
	defer tlsSwitch.Stop()

	if panelCfg.TLS.Enabled {
		info := tlsSwitch.Info()
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

	guardSwitch, err := reload.NewGuardSwitch(panelCfg, cfg.JWT.Secret, panelHandler, logger)
	if err != nil {
		logger.Fatal("Failed to build panel access gate", zap.Error(err))
	}

	// The listen address comes from the panel access config; the legacy
	// server.host/server.port pair is only used when the gate is switched off.
	addressFor := func(pc *config.PanelAccessConfig) string {
		if !pc.Enabled {
			return fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		}
		return pc.ListenAddr()
	}

	// The rebinder owns the listening socket. A port change opens the new one,
	// proves it answers and only then stops the old one accepting - in that
	// order, always, because a panel that closed its only door before opening
	// another has locked its operator out of the machine they administer.
	rebinder, err := reload.NewRebinder(reload.RebinderOptions{
		Handler:      guardSwitch,
		TLSConfig:    tlsSwitch.TLSConfig(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		Address:      addressFor,
		Logger:       logger,
	})
	if err != nil {
		logger.Fatal("Failed to build the panel listener", zap.Error(err))
	}

	// Registration order is commit order, and rollback runs it backwards. The
	// state file goes first because it is the cheapest thing to undo; the
	// listener goes last because it is the only one that touches the network.
	supervisor.Register(reload.NewStateFile(logger))
	supervisor.Register(tlsSwitch)
	supervisor.Register(guardSwitch)
	supervisor.Register(rebinder)

	// What "reachable" is proven against: the listener answers a real request
	// over a real socket, and the new gate admits a request identical to the
	// one that asked for the change. A change that fails either is undone by
	// this process, within seconds, without anybody having to notice.
	supervisor.SetProbe(reload.Probes{rebinder, guardSwitch})
	supervisor.SetCertificateReloader(tlsSwitch.Reload)
	supervisor.SetAudit(panelReloadAuditor(auditService, logger))

	// The certificate manager resolved the paths and the source the panel
	// actually ended up with. Adopting publishes that to everything holding a
	// copy of the configuration, the settings service included.
	supervisor.Adopt(panelCfg)

	// Configuration files are edited by hand and by the installer, and SIGHUP is
	// the first thing every operator tries. Both reach the same pipeline the API
	// does, so there is exactly one place where a configuration becomes live.
	watcher := reload.NewWatcher(supervisor, reload.WatcherOptions{
		StateFile: panelCfg.StateFile,
		EnvFile:   config.EnvFile(),
		Logger:    logger,
	})
	go watcher.Run(runCtx)
	go watcher.WatchSignals(runCtx)

	// Print the access table once, the way aaPanel does, so an operator can copy
	// the URL out of the console before closing the session.
	if panelCfg.Enabled {
		fmt.Print(panelCfg.Banner())
	}

	if err := rebinder.Start(panelCfg); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
	logger.Info("Panel server started",
		zap.String("addr", rebinder.Addr()),
		zap.Bool("tls", rebinder.UsesTLS()),
		zap.String("scheme", panelCfg.Scheme()),
		zap.String("certificate_source", tlsSwitch.Source()),
		zap.String("state_file", panelCfg.StateFile),
		zap.String("env_file", config.EnvFile()),
	)

	// Graceful shutdown. The listener dying on its own is also an exit: a panel
	// whose socket has gone is not reachable, and pretending otherwise leaves a
	// process that looks healthy to systemd and answers nothing.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("Shutting down server...")
	case err := <-rebinder.Fatal():
		logger.Error("Shutting down: the panel listener failed", zap.Error(err))
	}

	cancelRun()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rebinder.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited properly")
}

// panelReloadAuditor records a reload that did not come through the API.
//
// A change made through the settings endpoint is already audited there, with
// the operator who made it. This covers the other three origins - an edited
// state file, an edited environment file, SIGHUP - which have no operator
// attached and would otherwise leave the panel's port or entrance changing with
// nothing in the trail to say why.
func panelReloadAuditor(auditService *service.AuditService, logger *zap.Logger) reload.AuditFunc {
	return func(ctx context.Context, req reload.Request, outcome *reload.Outcome) {
		if req.Origin == reload.OriginAPI {
			return
		}

		changes := make([]map[string]string, 0, len(outcome.Changes))
		for _, change := range outcome.Changes {
			changes = append(changes, map[string]string{
				"field": change.Field,
				"old":   change.Old,
				"new":   change.New,
			})
		}

		status := "success"
		if !outcome.Applied {
			status = "failure"
		}

		if auditService != nil {
			details := models.JSONMap{
				"origin":           string(req.Origin),
				"source":           req.Detail,
				"changes":          changes,
				"change_count":     len(outcome.Changes),
				"applied":          outcome.Applied,
				"rolled_back":      outcome.RolledBack,
				"access_url":       outcome.AccessURL,
				"restart_required": outcome.RestartRequired,
			}
			if outcome.RollbackReason != "" {
				details["rollback_reason"] = outcome.RollbackReason
			}

			if err := auditService.Log(ctx, uuid.Nil, nil,
				"panel.settings.reload", service.PanelSettingsAuditResource, nil,
				details, req.ClientIP, string(req.Origin), status); err != nil {
				logger.Error("panel reload: audit log write failed", zap.Error(err))
			}
		}

		logger.Info("panel configuration reloaded from outside the API",
			zap.String("origin", string(req.Origin)),
			zap.String("source", req.Detail),
			zap.Int("changes", len(outcome.Changes)),
			zap.Bool("applied", outcome.Applied),
			zap.Bool("rolled_back", outcome.RolledBack),
			zap.Strings("restart_required", outcome.RestartRequired),
		)
	}
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
