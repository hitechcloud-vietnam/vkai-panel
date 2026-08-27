package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type ClusterRepository struct {
	db *sqlx.DB
}

func NewClusterRepository(db *sqlx.DB) *ClusterRepository {
	return &ClusterRepository{db: db}
}

// Clusters
func (r *ClusterRepository) Create(ctx context.Context, cluster *models.Cluster) error {
	query := `
		INSERT INTO clusters (tenant_id, name, description, type, status, config)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		cluster.TenantID, cluster.Name, cluster.Description,
		cluster.Type, cluster.Status, cluster.Config,
	).Scan(&cluster.ID, &cluster.CreatedAt, &cluster.UpdatedAt)
}

func (r *ClusterRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Cluster, error) {
	var cluster models.Cluster
	err := r.db.GetContext(ctx, &cluster,
		"SELECT * FROM clusters WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (r *ClusterRepository) List(ctx context.Context, tenantID uuid.UUID) ([]models.Cluster, error) {
	var clusters []models.Cluster
	err := r.db.SelectContext(ctx, &clusters,
		"SELECT * FROM clusters WHERE tenant_id = $1 ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	return clusters, nil
}

func (r *ClusterRepository) Update(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateClusterRequest) (*models.Cluster, error) {
	cluster, err := r.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		cluster.Name = *req.Name
	}
	if req.Description != nil {
		cluster.Description = *req.Description
	}
	if req.Type != nil {
		cluster.Type = *req.Type
	}
	if req.Status != nil {
		cluster.Status = *req.Status
	}
	if req.Config != nil {
		cluster.Config = *req.Config
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE clusters SET name=$1, description=$2, type=$3, status=$4, config=$5, updated_at=NOW()
		 WHERE tenant_id=$6 AND id=$7`,
		cluster.Name, cluster.Description, cluster.Type, cluster.Status, cluster.Config,
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (r *ClusterRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM clusters WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// Cluster Nodes
func (r *ClusterRepository) AddNode(ctx context.Context, node *models.ClusterNode) error {
	query := `
		INSERT INTO cluster_nodes (cluster_id, server_id, role, status, weight, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		node.ClusterID, node.ServerID, node.Role,
		node.Status, node.Weight, node.Metadata,
	).Scan(&node.ID, &node.CreatedAt, &node.UpdatedAt)
}

func (r *ClusterRepository) GetNodeByID(ctx context.Context, id uuid.UUID) (*models.ClusterNode, error) {
	var node models.ClusterNode
	err := r.db.GetContext(ctx, &node,
		"SELECT * FROM cluster_nodes WHERE id = $1",
		id,
	)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (r *ClusterRepository) ListNodes(ctx context.Context, clusterID uuid.UUID) ([]models.ClusterNode, error) {
	var nodes []models.ClusterNode
	err := r.db.SelectContext(ctx, &nodes,
		"SELECT * FROM cluster_nodes WHERE cluster_id = $1 ORDER BY role, server_id",
		clusterID,
	)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (r *ClusterRepository) UpdateNode(ctx context.Context, id uuid.UUID, req *models.UpdateClusterNodeRequest) (*models.ClusterNode, error) {
	node, err := r.GetNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Role != nil {
		node.Role = *req.Role
	}
	if req.Status != nil {
		node.Status = *req.Status
	}
	if req.Weight != nil {
		node.Weight = *req.Weight
	}
	if req.Metadata != nil {
		node.Metadata = *req.Metadata
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE cluster_nodes SET role=$1, status=$2, weight=$3, metadata=$4, updated_at=NOW()
		 WHERE id=$5`,
		node.Role, node.Status, node.Weight, node.Metadata, id,
	)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (r *ClusterRepository) UpdateNodeHeartbeat(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE cluster_nodes SET last_heartbeat = NOW() WHERE id = $1",
		id,
	)
	return err
}

func (r *ClusterRepository) RemoveNode(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM cluster_nodes WHERE id = $1",
		id,
	)
	return err
}

// Load Balancers
func (r *ClusterRepository) CreateLoadBalancer(ctx context.Context, lb *models.LoadBalancer) error {
	query := `
		INSERT INTO load_balancers (tenant_id, cluster_id, name, type, listen_port, backend_port, algorithm, config, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		lb.TenantID, lb.ClusterID, lb.Name, lb.Type,
		lb.ListenPort, lb.BackendPort, lb.Algorithm, lb.Config, lb.IsActive,
	).Scan(&lb.ID, &lb.CreatedAt, &lb.UpdatedAt)
}

func (r *ClusterRepository) GetLoadBalancerByID(ctx context.Context, tenantID, id uuid.UUID) (*models.LoadBalancer, error) {
	var lb models.LoadBalancer
	err := r.db.GetContext(ctx, &lb,
		"SELECT * FROM load_balancers WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &lb, nil
}

func (r *ClusterRepository) ListLoadBalancers(ctx context.Context, tenantID uuid.UUID, clusterID *uuid.UUID) ([]models.LoadBalancer, error) {
	var lbs []models.LoadBalancer
	var err error
	if clusterID != nil {
		err = r.db.SelectContext(ctx, &lbs,
			"SELECT * FROM load_balancers WHERE tenant_id = $1 AND cluster_id = $2 ORDER BY name",
			tenantID, *clusterID,
		)
	} else {
		err = r.db.SelectContext(ctx, &lbs,
			"SELECT * FROM load_balancers WHERE tenant_id = $1 ORDER BY name",
			tenantID,
		)
	}
	if err != nil {
		return nil, err
	}
	return lbs, nil
}

func (r *ClusterRepository) UpdateLoadBalancer(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateLoadBalancerRequest) (*models.LoadBalancer, error) {
	lb, err := r.GetLoadBalancerByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		lb.Name = *req.Name
	}
	if req.Type != nil {
		lb.Type = *req.Type
	}
	if req.ListenPort != nil {
		lb.ListenPort = *req.ListenPort
	}
	if req.BackendPort != nil {
		lb.BackendPort = *req.BackendPort
	}
	if req.Algorithm != nil {
		lb.Algorithm = *req.Algorithm
	}
	if req.Config != nil {
		lb.Config = *req.Config
	}
	if req.IsActive != nil {
		lb.IsActive = *req.IsActive
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE load_balancers SET name=$1, type=$2, listen_port=$3, backend_port=$4, algorithm=$5, config=$6, is_active=$7, updated_at=NOW()
		 WHERE tenant_id=$8 AND id=$9`,
		lb.Name, lb.Type, lb.ListenPort, lb.BackendPort, lb.Algorithm, lb.Config, lb.IsActive,
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return lb, nil
}

func (r *ClusterRepository) DeleteLoadBalancer(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM load_balancers WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// HA Pairs
func (r *ClusterRepository) CreateHAPair(ctx context.Context, ha *models.HAPair) error {
	query := `
		INSERT INTO ha_pairs (tenant_id, name, primary_id, secondary_id, virtual_ip, status, config)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		ha.TenantID, ha.Name, ha.PrimaryID, ha.SecondaryID,
		ha.VirtualIP, ha.Status, ha.Config,
	).Scan(&ha.ID, &ha.CreatedAt, &ha.UpdatedAt)
}

func (r *ClusterRepository) GetHAPairByID(ctx context.Context, tenantID, id uuid.UUID) (*models.HAPair, error) {
	var ha models.HAPair
	err := r.db.GetContext(ctx, &ha,
		"SELECT * FROM ha_pairs WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &ha, nil
}

func (r *ClusterRepository) ListHAPairs(ctx context.Context, tenantID uuid.UUID) ([]models.HAPair, error) {
	var has []models.HAPair
	err := r.db.SelectContext(ctx, &has,
		"SELECT * FROM ha_pairs WHERE tenant_id = $1 ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	return has, nil
}

func (r *ClusterRepository) UpdateHAPair(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateHAPairRequest) (*models.HAPair, error) {
	ha, err := r.GetHAPairByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		ha.Name = *req.Name
	}
	if req.VirtualIP != nil {
		ha.VirtualIP = *req.VirtualIP
	}
	if req.Status != nil {
		ha.Status = *req.Status
	}
	if req.Config != nil {
		ha.Config = *req.Config
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE ha_pairs SET name=$1, virtual_ip=$2, status=$3, config=$4, updated_at=NOW()
		 WHERE tenant_id=$5 AND id=$6`,
		ha.Name, ha.VirtualIP, ha.Status, ha.Config, tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return ha, nil
}

func (r *ClusterRepository) TriggerFailover(ctx context.Context, tenantID, id uuid.UUID) error {
	ha, err := r.GetHAPairByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	// Swap primary and secondary
	ha.PrimaryID, ha.SecondaryID = ha.SecondaryID, ha.PrimaryID
	ha.Status = "failover"

	_, err = r.db.ExecContext(ctx,
		`UPDATE ha_pairs SET primary_id=$1, secondary_id=$2, status=$3, last_failover=NOW(), updated_at=NOW()
		 WHERE tenant_id=$4 AND id=$5`,
		ha.PrimaryID, ha.SecondaryID, ha.Status, tenantID, id,
	)
	return err
}

func (r *ClusterRepository) DeleteHAPair(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM ha_pairs WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}
