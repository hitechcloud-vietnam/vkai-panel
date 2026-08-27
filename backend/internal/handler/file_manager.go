package handler

import (
	"io"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
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
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "Permissions changed"})
}

func (h *FileManagerHandler) Upload(c *gin.Context) {
	destPath := c.PostForm("path")
	if destPath == "" {
		utils.BadRequest(c, "Path is required")
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "No file uploaded")
		return
	}
	defer file.Close()

	if err := h.fileManager.Upload(c.Request.Context(), destPath, file.(io.Reader)); err != nil {
		utils.InternalError(c, err.Error())
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

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		utils.NotFound(c, "File not found")
		return
	}

	c.File(filePath)
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
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, files)
}

func (h *FileManagerHandler) GetDiskUsage(c *gin.Context) {
	path := c.DefaultQuery("path", "/")

	usage, err := h.fileManager.GetDiskUsage(c.Request.Context(), path)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}

	utils.Success(c, usage)
}
