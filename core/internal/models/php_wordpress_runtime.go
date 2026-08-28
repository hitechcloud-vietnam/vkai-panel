package models

// Models for the per-site PHP pool settings, the WordPress runtime identity and
// the staging environments.
//
// The nullable columns are pointers or sql.Null* on purpose. applied_php_version
// being NULL is a real state that means something specific - "the settings were
// recorded but nothing has ever reached the disk" - and collapsing it to the
// empty string makes it indistinguishable from "the apply succeeded and wrote
// an empty version", which is how a panel comes to report a state it never
// verified.

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// PHPPoolSettings is one site's PHP settings: what was asked for, and what
// actually reached the pool file.
type PHPPoolSettings struct {
	PoolID   string `json:"pool_id"`
	TenantID string `json:"tenant_id"`

	// The four the panel promises reach the pool file.
	MemoryLimit       string   `json:"memory_limit"`
	MaxExecutionTime  int      `json:"max_execution_time"`
	UploadMaxFilesize string   `json:"upload_max_filesize"`
	Extensions        []string `json:"extensions"`

	PostMaxSize       string   `json:"post_max_size"`
	MaxInputTime      int      `json:"max_input_time"`
	MaxFileUploads    int      `json:"max_file_uploads"`
	Timezone          string   `json:"timezone"`
	DisplayErrors     bool     `json:"display_errors"`
	DisabledFunctions []string `json:"disabled_functions"`
	OpenBasedir       []string `json:"open_basedir"`

	// What is in force on the host, as opposed to what was asked for above.
	AppliedPHPVersion sql.NullString `json:"-"`
	PoolFile          sql.NullString `json:"-"`
	SocketPath        sql.NullString `json:"-"`
	LastAppliedAt     sql.NullTime   `json:"-"`
	LastError         sql.NullString `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AppliedState is the JSON view of the applied_* columns, so an API caller can
// see the gap between intent and reality without decoding sql.Null*.
type AppliedState struct {
	PHPVersion string `json:"php_version,omitempty"`
	PoolFile   string `json:"pool_file,omitempty"`
	SocketPath string `json:"socket_path,omitempty"`
	AppliedAt  string `json:"applied_at,omitempty"`
	LastError  string `json:"last_error,omitempty"`
	// InForce is false when nothing has ever been written for this pool.
	InForce bool `json:"in_force"`
}

// Applied renders the applied_* columns for an API response.
func (s *PHPPoolSettings) Applied() AppliedState {
	state := AppliedState{
		PHPVersion: s.AppliedPHPVersion.String,
		PoolFile:   s.PoolFile.String,
		SocketPath: s.SocketPath.String,
		LastError:  s.LastError.String,
		InForce:    s.LastAppliedAt.Valid,
	}
	if s.LastAppliedAt.Valid {
		state.AppliedAt = s.LastAppliedAt.Time.UTC().Format(time.RFC3339)
	}
	return state
}

// WordPressRuntime is the system identity a WordPress site's commands run as.
type WordPressRuntime struct {
	SiteID   uuid.UUID `json:"site_id"`
	TenantID uuid.UUID `json:"tenant_id"`

	// RunAsUser is the unix user every WP-CLI command for this site runs as,
	// and the user its PHP-FPM pool runs as. It is never root: the database
	// refuses that with a CHECK constraint and internal/wpcli refuses it again
	// at the point of exec.
	RunAsUser  string `json:"run_as_user"`
	RunAsGroup string `json:"run_as_group"`

	PHPVersion       sql.NullString `json:"-"`
	InstalledVersion sql.NullString `json:"-"`
	LastRanAs        sql.NullString `json:"-"`
	LastCommand      sql.NullString `json:"-"`
	LastRanAt        sql.NullTime   `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RuntimeView is the JSON view of a runtime row.
type RuntimeView struct {
	RunAsUser        string `json:"run_as_user"`
	RunAsGroup       string `json:"run_as_group"`
	PHPVersion       string `json:"php_version,omitempty"`
	InstalledVersion string `json:"installed_wordpress_version,omitempty"`
	LastRanAs        string `json:"last_ran_as,omitempty"`
	LastCommand      string `json:"last_command,omitempty"`
	LastRanAt        string `json:"last_ran_at,omitempty"`
}

// View renders a runtime row for an API response.
func (r *WordPressRuntime) View() RuntimeView {
	view := RuntimeView{
		RunAsUser:        r.RunAsUser,
		RunAsGroup:       r.RunAsGroup,
		PHPVersion:       r.PHPVersion.String,
		InstalledVersion: r.InstalledVersion.String,
		LastRanAs:        r.LastRanAs.String,
		LastCommand:      r.LastCommand.String,
	}
	if r.LastRanAt.Valid {
		view.LastRanAt = r.LastRanAt.Time.UTC().Format(time.RFC3339)
	}
	return view
}

// WordPressStaging is one staging environment and the record of the last push.
type WordPressStaging struct {
	ID               uuid.UUID `json:"id"`
	TenantID         uuid.UUID `json:"tenant_id"`
	ProductionSiteID uuid.UUID `json:"production_site_id"`

	StagingDomain     string `json:"staging_domain"`
	StagingPath       string `json:"staging_path"`
	StagingURL        string `json:"staging_url"`
	StagingDBName     string `json:"staging_db_name"`
	StagingDBUser     string `json:"staging_db_user"`
	StagingDBPassword string `json:"-"`
	StagingDBHost     string `json:"staging_db_host"`

	Status        string `json:"status"`
	BlockIndexing bool   `json:"block_indexing"`

	LastCloneAt sql.NullTime `json:"-"`
	LastPushAt  sql.NullTime `json:"-"`
	// LastPushDatabase is the decision that was made on the last push. NULL
	// means no push has ever run; it is never a default.
	LastPushDatabase sql.NullString `json:"-"`
	LastPushBackup   sql.NullString `json:"-"`
	LastPushDBBackup sql.NullString `json:"-"`
	LastError        sql.NullString `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StagingHistory is the JSON view of the last clone and push.
type StagingHistory struct {
	LastCloneAt      string `json:"last_clone_at,omitempty"`
	LastPushAt       string `json:"last_push_at,omitempty"`
	LastPushDatabase string `json:"last_push_database,omitempty"`
	LastPushBackup   string `json:"last_push_files_backup,omitempty"`
	LastPushDBBackup string `json:"last_push_database_backup,omitempty"`
	LastError        string `json:"last_error,omitempty"`
}

// History renders the audit columns for an API response.
func (s *WordPressStaging) History() StagingHistory {
	history := StagingHistory{
		LastPushDatabase: s.LastPushDatabase.String,
		LastPushBackup:   s.LastPushBackup.String,
		LastPushDBBackup: s.LastPushDBBackup.String,
		LastError:        s.LastError.String,
	}
	if s.LastCloneAt.Valid {
		history.LastCloneAt = s.LastCloneAt.Time.UTC().Format(time.RFC3339)
	}
	if s.LastPushAt.Valid {
		history.LastPushAt = s.LastPushAt.Time.UTC().Format(time.RFC3339)
	}
	return history
}
