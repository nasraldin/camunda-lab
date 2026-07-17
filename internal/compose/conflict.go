package compose

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var containerNameConflictRE = regexp.MustCompile(`container name "(/[^"]+)" is already in use`)

// ParseNameConflicts extracts Docker container names from compose name-conflict errors.
func ParseNameConflicts(msg string) []string {
	matches := containerNameConflictRE.FindAllStringSubmatch(msg, -1)
	seen := map[string]bool{}
	var names []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimPrefix(m[1], "/")
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// ContainerInfo is a small snapshot from docker inspect.
type ContainerInfo struct {
	ID      string
	Name    string
	State   string
	Project string
	Image   string
}

// InspectContainerByName returns container metadata for a Docker name (with or without leading /).
func InspectContainerByName(name string) (*ContainerInfo, error) {
	name = strings.TrimPrefix(name, "/")
	out, err := exec.Command(
		"docker", "inspect", "--format",
		"{{.Id}}\t{{.State.Status}}\t{{index .Config.Labels \"com.docker.compose.project\"}}\t{{.Config.Image}}",
		name,
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", name, err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) < 4 {
		return nil, fmt.Errorf("unexpected inspect output for %s", name)
	}
	return &ContainerInfo{
		ID:      parts[0],
		Name:    name,
		State:   parts[1],
		Project: parts[2],
		Image:   parts[3],
	}, nil
}

// IsCamundaRelated reports whether a container image or compose project looks Camunda-lab related.
func IsCamundaRelated(image, project string) bool {
	if project == "camunda-lab" || strings.HasPrefix(project, "camunda") {
		return true
	}
	img := strings.ToLower(image)
	keywords := []string{
		"camunda", "zeebe", "keycloak", "elasticsearch", "elastic",
		"postgres", "confluentinc", "optimize", "operate", "tasklist",
		"connectors", "identity", "mailpit", "kibana",
	}
	for _, k := range keywords {
		if strings.Contains(img, k) {
			return true
		}
	}
	return false
}

// CanSafelyRemove reports whether a leftover container can be removed for a new lab start.
func CanSafelyRemove(c *ContainerInfo, ourProject string) bool {
	if c == nil {
		return false
	}
	if c.Project == ourProject {
		return true
	}
	switch c.State {
	case "exited", "created", "dead", "paused":
		return IsCamundaRelated(c.Image, c.Project)
	case "running":
		return c.Project == ourProject
	default:
		return false
	}
}

// RemoveContainer force-removes a container by name or ID.
func RemoveContainer(nameOrID string) error {
	out, err := exec.Command("docker", "rm", "-f", nameOrID).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("docker rm %s: %s", nameOrID, msg)
	}
	return nil
}

// KnownFixedNames are common global container_name values in Camunda compose files.
var KnownFixedNames = []string{
	"postgres", "keycloak", "elasticsearch", "zeebe", "operate", "tasklist",
	"connectors", "optimize", "identity", "console", "web-modeler", "mailpit",
}
