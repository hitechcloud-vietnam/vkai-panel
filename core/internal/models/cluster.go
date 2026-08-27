package models

import (
	"time"

	"github.com/google/uuid"
)

// Cluster represents a cluster of servers
type Cluster struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	Type        string     `json:"type" db:"type"`
	Status      string     `json:"status" db:"status"`
	Config      JSONMap    `json:"config" db:"config"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// ClusterNode represents a node in a cluster
type ClusterNode struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	ClusterID   uuid.UUID  `json:"cluster_id" db:"cluster_id"`
	ServerID    uuid.UUID  `json:"server_id" db:"server_id"`
	Role        string     `json:"role" db:"role"`
	Status      string     `json:"status" db:"status"`
	Weight      int        `json:"weight" db:"weight"`
	Metadata    JSONMap    `json:"metadata" db:"metadata"`
	LastHeartbeat *time.Time `json:"last_heartbeat" db:"last_heartbeat"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// LoadBalancer represents a load balancer configuration
type LoadBalancer struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ClusterID   uuid.UUID  `json:"cluster_id" db:"cluster_id"`
	Name        string     `json:"name" db:"name"`
	Type        string     `json:"type" db:"type"`
	ListenPort  int        `json:"listen_port" db:"listen_port"`
	BackendPort int        `json:"backend_port" db:"backend_port"`
	Algorithm   string     `json:"algorithm" db:"algorithm"`
	Config      JSONMap    `json:"config" db:"config"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// HAPair represents a high-availability pair
type HAPair struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	PrimaryID   uuid.UUID  `json:"primary_id" db:"primary_id"`
	SecondaryID uuid.UUID  `json:"secondary_id" db:"secondary_id"`
	VirtualIP   string     `json:"virtual_ip" db:"virtual_ip"`
	Status      string     `json:"status" db:"status"`
	Config      JSONMap    `json:"config" db:"config"`
	LastFailover *time.Time `json:"last_failover" db:"last_failover"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// CreateClusterRequest represents a request to create a cluster
type CreateClusterRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Type        string  `json:"type" binding:"required"`
	Config      JSONMap `json:"config"`
}

// UpdateClusterRequest represents a request to update a cluster
type UpdateClusterRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Type        *string  `json:"type"`
	Status      *string  `json:"status"`
	Config      *JSONMap `json:"config"`
}

// AddClusterNodeRequest represents a request to add a node to a cluster
type AddClusterNodeRequest struct {
	ServerID uuid.UUID `json:"server_id" binding:"required"`
	Role     string    `json:"role" binding:"required"`
	Weight   int       `json:"weight"`
	Metadata JSONMap   `json:"metadata"`
}

// UpdateClusterNodeRequest represents a request to update a cluster node
type UpdateClusterNodeRequest struct {
	Role     *string  `json:"role"`
	Status   *string  `json:"status"`
	Weight   *int     `json:"weight"`
	Metadata *JSONMap `json:"metadata"`
}

// CreateLoadBalancerRequest represents a request to create a load balancer
type CreateLoadBalancerRequest struct {
	ClusterID   uuid.UUID `json:"cluster_id" binding:"required"`
	Name        string    `json:"name" binding:"required"`
	Type        string    `json:"type" binding:"required"`
	ListenPort  int       `json:"listen_port" binding:"required"`
	BackendPort int       `json:"backend_port" binding:"required"`
	Algorithm   string    `json:"algorithm" binding:"required"`
	Config      JSONMap   `json:"config"`
}

// UpdateLoadBalancerRequest represents a request to update a load balancer
type UpdateLoadBalancerRequest struct {
	Name        *string  `json:"name"`
	Type        *string  `json:"type"`
	ListenPort  *int     `json:"listen_port"`
	BackendPort *int     `json:"backend_port"`
	Algorithm   *string  `json:"algorithm"`
	Config      *JSONMap `json:"config"`
	IsActive    *bool    `json:"is_active"`
}

// CreateHAPairRequest represents a request to create an HA pair
type CreateHAPairRequest struct {
	Name        string    `json:"name" binding:"required"`
	PrimaryID   uuid.UUID `json:"primary_id" binding:"required"`
	SecondaryID uuid.UUID `json:"secondary_id" binding:"required"`
	VirtualIP   string    `json:"virtual_ip" binding:"required"`
	Config      JSONMap   `json:"config"`
}

// UpdateHAPairRequest represents a request to update an HA pair
type UpdateHAPairRequest struct {
	Name      *string  `json:"name"`
	VirtualIP *string  `json:"virtual_ip"`
	Status    *string  `json:"status"`
	Config    *JSONMap `json:"config"`
}
