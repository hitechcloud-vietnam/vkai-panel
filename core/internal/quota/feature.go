package quota

// The feature names a hosting package can switch on or off.
//
// They live here rather than as free strings in the migration so that a package
// cannot be sold with a feature the panel has never heard of, and so that the
// interface can render the list without guessing. A feature not present in a
// package's JSON object is NOT allowed - see Assignment.FeatureAllowed.
const (
	FeatureSSH           = "ssh"
	FeatureCron          = "cron"
	FeatureDNS           = "dns"
	FeatureMailServer    = "mail_server"
	FeatureDocker        = "docker"
	FeatureNodeApps      = "node_apps"
	FeatureWordPress     = "wordpress"
	FeatureGitDeployment = "git_deployment"
	FeatureBackupRestore = "backup_restore"
	FeatureWAF           = "waf"
	FeatureStaging       = "staging"
)

// KnownFeatures is the complete list, in the order the interface shows them.
var KnownFeatures = []string{
	FeatureSSH,
	FeatureCron,
	FeatureDNS,
	FeatureMailServer,
	FeatureDocker,
	FeatureNodeApps,
	FeatureWordPress,
	FeatureGitDeployment,
	FeatureBackupRestore,
	FeatureWAF,
	FeatureStaging,
}

// KnownFeature reports whether name is a feature this panel understands.
func KnownFeature(name string) bool {
	for _, f := range KnownFeatures {
		if f == name {
			return true
		}
	}
	return false
}
