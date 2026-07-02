package router

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/yourorg/chunked-upload/internal/api/handlers"
	"github.com/yourorg/chunked-upload/internal/api/middleware"
	"github.com/yourorg/chunked-upload/internal/config"
	"github.com/yourorg/chunked-upload/internal/services/upload"
)

// New creates and returns a fully wired Gin engine.
func New(cfg *config.Config, uploadSvc *upload.Service) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	// ── CORS – explicit allow-list, never wildcard ─────────────────────────
	corsConfig := cors.Config{
		AllowOrigins:     cfg.App.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Content-Length", "Accept", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           43200, // 12 hours
	}
	r.Use(cors.New(corsConfig))

	// ── Swagger ───────────────────────────────────────────────────────────
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── Health ────────────────────────────────────────────────────────────
	r.GET("/health", handlers.Health)

	// ── API v1 ────────────────────────────────────────────────────────────
	h := handlers.New(uploadSvc)

	api := r.Group("/api/v1")
	api.Use(middleware.GeneralRateLimit())
	{
		upload := api.Group("/upload")
		{
			upload.POST("/init", h.InitUpload)

			// Chunk endpoint has its own tighter rate limit and size cap
			upload.POST("/chunk",
				middleware.ChunkUploadRateLimit(),
				handlers.MaxChunkSizeMiddleware(cfg.Upload.ChunkSizeBytes+1024), // +1KB tolerance
				h.UploadChunk,
			)

			upload.POST("/complete", h.CompleteUpload)
			upload.GET("/status", h.UploadStatus)
		}
	}

	return r
}
