package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type ServerHandler struct {
	serverService *service.ServerService
	logger        *zap.Logger
}

func NewServerHandler(serverService *service.ServerService, logger *zap.Logger) *ServerHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ServerHandler{serverService: serverService, logger: logger}
}

func (h *ServerHandler) Create(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req models.CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	server, err := h.serverService.Create(c.Request.Context(), tenantID, req)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Created(c, server)
}

func (h *ServerHandler) Get(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, ok := h.resolveServerID(c)
	if !ok {
		return
	}

	server, err := h.serverService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		utils.NotFound(c, "Server not found")
		return
	}

	utils.Success(c, h.withLocalNode(c, []models.Server{*server})[0])
}

func (h *ServerHandler) List(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		params.Page = 1
		params.PerPage = 20
	}
	params.Normalize()

	servers, total, err := h.serverService.ListByTenant(c.Request.Context(), tenantID, params.Page, params.PerPage)
	if err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Paginated(c, h.withLocalNode(c, servers), total, params.Page, params.PerPage)
}

func (h *ServerHandler) Update(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, ok := h.resolveServerID(c)
	if !ok {
		return
	}

	server, err := h.serverService.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		utils.NotFound(c, "Server not found")
		return
	}

	var req struct {
		Hostname  string   `json:"hostname"`
		IPAddress string   `json:"ip_address"`
		SSHPort   int      `json:"ssh_port"`
		Location  string   `json:"location"`
		Tags      []string `json:"tags"`
		Role      string   `json:"role"`
		Status    string   `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "Invalid request body")
		return
	}

	if req.Hostname != "" {
		server.Hostname = req.Hostname
	}
	if req.IPAddress != "" {
		server.IPAddress = req.IPAddress
	}
	if req.SSHPort > 0 {
		server.SSHPort = req.SSHPort
	}
	if req.Location != "" {
		server.Location = req.Location
	}
	if req.Tags != nil {
		server.Tags = req.Tags
	}
	if req.Role != "" {
		server.Role = req.Role
	}
	if req.Status != "" {
		server.Status = req.Status
	}

	if err := h.serverService.Update(c.Request.Context(), server); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.Success(c, server)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	id, ok := h.resolveServerID(c)
	if !ok {
		return
	}

	// The machine the panel is running on cannot be removed from its own
	// inventory. Deleting the row would not stop the panel running there; it
	// would only leave the sites, databases and certificates on this host with
	// no node to hang off, and the next start would register it again anyway.
	if localID, err := h.serverService.LocalNodeID(c.Request.Context()); err == nil && localID == id {
		utils.Conflict(c, "This node is the machine the panel is installed on and cannot be deleted")
		return
	}

	if err := h.serverService.Delete(c.Request.Context(), tenantID, id); err != nil {
		utils.InternalError(c, err)
		return
	}

	utils.NoContent(c)
}

func (h *ServerHandler) GetMetrics(c *gin.Context) {
	id, ok := h.resolveServerID(c)
	if !ok {
		return
	}

	metrics, err := h.serverService.GetMetrics(c.Request.Context(), id)
	if err != nil {
		utils.NotFound(c, "Metrics not found")
		return
	}

	utils.Success(c, metrics)
}

// ============================================================
// THE PANEL HOST AMONG THE SERVERS
//
// The machine the panel is installed on is a node like any other, so it is
// listed and fetched through the same routes. Two things are added here and
// nothing is changed:
//
//   - the identifier "local" in place of a UUID, so a client that does not yet
//     know the node id can ask for the machine it is talking to;
//   - a "local_node" object alongside the server, present only on that node.
//
// Both are additive. The server object keeps every field it had, in the same
// place, so an existing client sees exactly what it saw before. That is also
// why no new route was introduced: a path segment beside /servers/:id would
// have to be registered in the router, and the alias needs nothing.
// ============================================================

// localServerAlias is the identifier that means "the machine this panel is
// installed on" wherever a server id is accepted.
const localServerAlias = "local"

// serverView is a server plus, when it is the panel host, what is known about
// that claim. The embedded pointer promotes every field of models.Server into
// the same JSON object it produced before, so this is an added key rather than
// a changed shape.
type serverView struct {
	*models.Server
	LocalNode *service.LocalNodeStatus `json:"local_node,omitempty"`
}

// resolveServerID reads the :id parameter, accepting the "local" alias. It
// writes the error response itself and returns false when it did, so callers
// stay a single early return.
func (h *ServerHandler) resolveServerID(c *gin.Context) (uuid.UUID, bool) {
	raw := strings.TrimSpace(c.Param("id"))
	if strings.EqualFold(raw, localServerAlias) {
		id, err := h.serverService.LocalNodeID(c.Request.Context())
		if err != nil {
			utils.NotFound(c, "This panel host is not registered as a managed node")
			return uuid.Nil, false
		}
		return id, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		utils.BadRequest(c, "Invalid server ID")
		return uuid.Nil, false
	}
	return id, true
}

// withLocalNode attaches the local-node status to whichever of these servers is
// the machine this panel runs on.
//
// The status is fetched once for the whole page rather than per row, because at
// most one row can be it. A panel that cannot answer - no local node, an
// unmigrated database, an unreadable identity - attaches nothing: the local
// node is extra information about a server, never a precondition for listing
// it.
func (h *ServerHandler) withLocalNode(c *gin.Context, servers []models.Server) []serverView {
	views := make([]serverView, len(servers))
	for i := range servers {
		views[i] = serverView{Server: &servers[i]}
	}

	status, err := h.serverService.LocalNodeStatus(c.Request.Context())
	if err != nil {
		h.logger.Debug("Servers: the local node could not be described", zap.Error(err))
		return views
	}
	if status == nil || !status.Registered {
		return views
	}
	for i := range views {
		if views[i].Server.ID == status.NodeID {
			views[i].LocalNode = status
			break
		}
	}
	return views
}
