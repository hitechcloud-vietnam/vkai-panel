package models

import (
	"time"

	"github.com/google/uuid"
)

// The field names and db tags in this file are the ones migration 014 defines.
// Where the Go code and the schema disagreed, the schema won: it is the shape
// the foreign keys, the indexes and the panel's clusters page were all written
// against, and it is the definition that has been verified against a real
// PostgreSQL 16.

// Cluster represents a cluster of servers
type Cluster struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Type        string    `json:"type" db:"type"`
	Status      string    `json:"status" db:"status"`
	Config      JSONMap   `json:"config" db:"config"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ClusterNode represents a node in a cluster
type ClusterNode struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ClusterID uuid.UUID `json:"cluster_id" db:"cluster_id"`
	ServerID  uuid.UUID `json:"server_id" db:"server_id"`
	Role      string    `json:"role" db:"role"`
	Status    string    `json:"status" db:"status"`
	// AddNode never writes ip_address, so every row has it NULL until an
	// operator fills it in. It has to be a pointer or listing nodes fails.
	IPAddress     *string    `json:"ip_address" db:"ip_address"`
	Port          int        `json:"port" db:"port"`
	Weight        int        `json:"weight" db:"weight"`
	Metadata      JSONMap    `json:"metadata" db:"metadata"`
	LastHeartbeat *time.Time `json:"last_heartbeat" db:"last_heartbeat"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

// LoadBalancer represents a load balancer configuration
type LoadBalancer struct {
	ID       uuid.UUID `json:"id" db:"id"`
	TenantID uuid.UUID `json:"tenant_id" db:"tenant_id"`
	// cluster_id is ON DELETE SET NULL, so a load balancer outlives its
	// cluster with a NULL here.
	ClusterID   *uuid.UUID `json:"cluster_id" db:"cluster_id"`
	Name        string     `json:"name" db:"name"`
	Type        string     `json:"type" db:"type"`
	Algorithm   string     `json:"algorithm" db:"algorithm"`
	Status      string     `json:"status" db:"status"`
	ListenPort  int        `json:"listen_port" db:"listen_port"`
	SSLPort     int        `json:"ssl_port" db:"ssl_port"`
	SSLEnabled  bool       `json:"ssl_enabled" db:"ssl_enabled"`
	Config      JSONMap    `json:"config" db:"config"`
	HealthCheck JSONMap    `json:"health_check" db:"health_check"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// HAPair represents a high-availability pair
type HAPair struct {
	ID       uuid.UUID `json:"id" db:"id"`
	TenantID uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name     string    `json:"name" db:"name"`
	// These are foreign keys into servers, which is why the schema calls them
	// primary_server_id and secondary_server_id.
	PrimaryServerID   uuid.UUID `json:"primary_server_id" db:"primary_server_id"`
	SecondaryServerID uuid.UUID `json:"secondary_server_id" db:"secondary_server_id"`
	VirtualIP         string    `json:"virtual_ip" db:"virtual_ip"`
	Status            string    `json:"status" db:"status"`
	FailoverMode      string    `json:"failover_mode" db:"failover_mode"`
	// Nothing writes last_sync yet, so it is NULL on every row.
	LastSync     *time.Time `json:"last_sync" db:"last_sync"`
	LastFailover *time.Time `json:"last_failover" db:"last_failover"`
	Config       JSONMap    `json:"config" db:"config"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
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
	ClusterID  uuid.UUID `json:"cluster_id" binding:"required"`
	Name       string    `json:"name" binding:"required"`
	Type       string    `json:"type" binding:"required"`
	ListenPort int       `json:"listen_port" binding:"required"`
	Algorithm  string    `json:"algorithm" binding:"required"`
	SSLPort    *int      `json:"ssl_port"`
	SSLEnabled *bool     `json:"ssl_enabled"`
	Config     JSONMap   `json:"config"`
}

// UpdateLoadBalancerRequest represents a request to update a load balancer
type UpdateLoadBalancerRequest struct {
	Name       *string  `json:"name"`
	Type       *string  `json:"type"`
	Algorithm  *string  `json:"algorithm"`
	Status     *string  `json:"status"`
	ListenPort *int     `json:"listen_port"`
	SSLPort    *int     `json:"ssl_port"`
	SSLEnabled *bool    `json:"ssl_enabled"`
	Config     *JSONMap `json:"config"`
}

// CreateHAPairRequest represents a request to create an HA pair
type CreateHAPairRequest struct {
	Name              string    `json:"name" binding:"required"`
	PrimaryServerID   uuid.UUID `json:"primary_server_id" binding:"required"`
	SecondaryServerID uuid.UUID `json:"secondary_server_id" binding:"required"`
	VirtualIP         string    `json:"virtual_ip" binding:"required"`
	FailoverMode      *string   `json:"failover_mode"`
	Config            JSONMap   `json:"config"`
}

// UpdateHAPairRequest represents a request to update an HA pair
type UpdateHAPairRequest struct {
	Name         *string  `json:"name"`
	VirtualIP    *string  `json:"virtual_ip"`
	Status       *string  `json:"status"`
	FailoverMode *string  `json:"failover_mode"`
	Config       *JSONMap `json:"config"`
}
