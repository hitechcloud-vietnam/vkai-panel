package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// isPathError reports whether the error came from the file manager's jail
// check, so the handler can answer 400 instead of leaking a 500 with details.
func isPathError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "outside the allowed root") ||
		strings.Contains(msg, "not accessible") ||
		strings.Contains(msg, "invalid path")
}

// respondFileError maps a file manager error onto a status code.
func respondFileError(c *gin.Context, err error) {
	if isPathError(err) {
		utils.BadRequest(c, "Invalid path")
		return
	}
	utils.InternalError(c, err.Error())
}

type FileManagerHandler struct {
	fileManager *service.FileManager
}

func NewFileManagerHandler(fileManager *service.FileManager) *FileManagerHandler {
	return &FileManagerHandler{fileManager: fileManager}
}

func (h *FileManagerHandler) ListFiles(c *gin.Context) {
	dirPath := c.DefaultQuery("path", "/")

	files, err := h.fileManager.ListFiles(c.Request.Context(), dirPath)
	if err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, files)
}

func (h *FileManagerHandler) ReadFile(c *gin.Context) {
	filePath := c.Query("path")
	if filePath == "" {
		utils.BadRequest(c, "Path is required")
		return
	}

	content, err := h.fileManager.ReadFile(c.Request.Context(), filePath)
	if err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{
		"path":    filePath,
		"content": string(content),
		"size":    len(content),
	})
}

func (h *FileManagerHandler) WriteFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.fileManager.WriteFile(c.Request.Context(), req.Path, []byte(req.Content)); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "File saved"})
}

func (h *FileManagerHandler) CreateDirectory(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.fileManager.CreateDirectory(c.Request.Context(), req.Path); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Created(c, gin.H{"message": "Directory created"})
}

func (h *FileManagerHandler) Delete(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.fileManager.Delete(c.Request.Context(), req.Path); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "Deleted"})
}

func (h *FileManagerHandler) Rename(c *gin.Context) {
	var req struct {
		OldPath string `json:"old_path" binding:"required"`
		NewPath string `json:"new_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.fileManager.Rename(c.Request.Context(), req.OldPath, req.NewPath); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "Renamed"})
}

func (h *FileManagerHandler) Copy(c *gin.Context) {
	var req struct {
		Src string `json:"src" binding:"required"`
		Dst string `json:"dst" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if err := h.fileManager.Copy(c.Request.Context(), req.Src, req.Dst); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "Copied"})
}

func (h *FileManagerHandler) ChangePermissions(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	mode, err := service.ParseFileMode(req.Mode)
	if err != nil {
		utils.BadRequest(c, "Invalid file mode")
		return
	}

	if err := h.fileManager.ChangePermissions(c.Request.Context(), req.Path, mode); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "Permissions changed"})
}

func (h *FileManagerHandler) Upload(c *gin.Context) {
	// Cap the whole request body before anything parses (and buffers) it.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxUploadBytes)

	destPath := c.PostForm("path")
	if destPath == "" {
		utils.BadRequest(c, "Path is required")
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.Error(c, http.StatusRequestEntityTooLarge, "No file uploaded or file too large")
		return
	}
	defer file.Close()

	if err := h.fileManager.Upload(c.Request.Context(), destPath, file); err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, gin.H{"message": "File uploaded"})
}

func (h *FileManagerHandler) Download(c *gin.Context) {
	filePath := c.Query("path")
	if filePath == "" {
		utils.BadRequest(c, "Path is required")
		return
	}

	// Downloads go through the same jail as every other file operation.
	safePath, err := h.fileManager.ResolvePath(filePath)
	if err != nil {
		utils.BadRequest(c, "Invalid path")
		return
	}

	info, err := os.Stat(safePath)
	if err != nil || info.IsDir() {
		utils.NotFound(c, "File not found")
		return
	}

	// Force a download so HTML/SVG in a web root cannot execute on the panel's
	// own origin.
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", "application/octet-stream")
	c.FileAttachment(safePath, filepath.Base(safePath))
}

func (h *FileManagerHandler) Search(c *gin.Context) {
	dir := c.DefaultQuery("path", "/")
	pattern := c.Query("pattern")
	if pattern == "" {
		utils.BadRequest(c, "Pattern is required")
		return
	}

	files, err := h.fileManager.SearchFiles(c.Request.Context(), dir, pattern)
	if err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, files)
}

func (h *FileManagerHandler) GetDiskUsage(c *gin.Context) {
	path := c.DefaultQuery("path", "/")

	usage, err := h.fileManager.GetDiskUsage(c.Request.Context(), path)
	if err != nil {
		respondFileError(c, err)
		return
	}

	utils.Success(c, usage)
}
