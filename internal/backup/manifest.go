package backup

// Manifest describes a backup archive.
type Manifest struct {
	Version         int               `json:"version"`
	CreatedAt       string            `json:"createdAt"`
	Lab             map[string]string `json:"lab"`
	IncludesSecrets bool              `json:"includesSecrets"`
	Files           []string          `json:"files"`
	AISecretKeys    []string          `json:"aiSecretKeys,omitempty"`
}
