package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type Router struct {
	engine         *gin.Engine
	authHandler    *AuthHandler
	serverHandler  *ServerHandler
	tenantHandler  *TenantHandler
	userHandler    *UserHandler
	healthHandler  *HealthHandler
	jwtManager     *auth.JWTManager
	logger         *zap.Logger
}

func NewRouter(
	authHandler *AuthHandler,
	serverHandler *ServerHandler,
	tenantHandler *TenantHandler,
	userHandler *UserHandler,
	healthHandler *HealthHandler,
	jwtManager *auth.JWTManager,
	logger *zap.Logger,
) *Router {
	engine := gin.New()

	return &Router{
		engine:        engine,
		authHandler:   authHandler,
		serverHandler: serverHandler,
		tenantHandler: tenantHandler,
		userHandler:   userHandler,
		healthHandler: healthHandler,
		jwtManager:    jwtManager,
		logger:        logger,
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
		// TODO: Add website routes

		// Databases
		// TODO: Add database routes

		// DNS
		// TODO: Add DNS routes

		// SSL
		// TODO: Add SSL routes

		// Docker
		// TODO: Add Docker routes

		// Files
		// TODO: Add file manager routes

		// Cron
		// TODO: Add cron routes

		// Firewall
		// TODO: Add firewall routes

		// Backups
		// TODO: Add backup routes

		// Monitoring
		// TODO: Add monitoring routes

		// Logs
		// TODO: Add log routes

		// Deployments
		// TODO: Add deployment routes
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
