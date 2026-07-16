package urls

import (
	"fmt"
	"strconv"

	"github.com/nasraldin/camunda-lab/internal/config"
)

// Entry is a named component URL for camunda urls / open / smoke.
type Entry struct {
	Name  string
	URL   string
	Notes string
}

// List returns component URLs for the active lab config.
//
// Ports follow Camunda's official docker-compose for each minor
// (camunda/camunda-distributions), not a single shared docs table:
//
//   - 8.5–8.7: separate Operate (8081) / Tasklist (8082); Zeebe HTTP gateway 8088
//   - 8.8: consolidated orchestration published on host 8088
//   - 8.9+: consolidated orchestration published on host 8080
//
// See https://docs.camunda.io/docs/8.7/self-managed/setup/deploy/local/docker-compose/
// and https://docs.camunda.io/docs/self-managed/quickstart/developer-quickstart/docker-compose/configuration.md
func List(cfg config.Config) []Entry {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}

	switch cfg.Profile {
	case "modeler":
		return []Entry{
			{Name: "web-modeler", URL: fmt.Sprintf("http://%s:8070", host), Notes: "demo/demo"},
		}
	case "full":
		return fullEntries(cfg.Version, host)
	default: // light
		return lightEntries(cfg.Version, host)
	}
}

func lightEntries(version, host string) []Entry {
	if isPre88(version) {
		return []Entry{
			{Name: "operate", URL: fmt.Sprintf("http://%s:8081", host), Notes: "demo/demo"},
			{Name: "tasklist", URL: fmt.Sprintf("http://%s:8082", host), Notes: "demo/demo"},
			{Name: "connectors", URL: fmt.Sprintf("http://%s:8085", host)},
			{Name: "zeebe-http", URL: fmt.Sprintf("http://%s:8088", host), Notes: "Zeebe gateway HTTP"},
			{Name: "rest", URL: fmt.Sprintf("http://%s:8088", host), Notes: "Desktop Modeler restAddress"},
			{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
			{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)},
		}
	}

	port := orchestrationHostPort(version)
	entries := orchestrationUI(host, port)
	entries = append(entries,
		Entry{Name: "connectors", URL: fmt.Sprintf("http://%s:8086", host)},
		Entry{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
	)
	if version != "8.10" {
		entries = append(entries, Entry{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)})
	}
	return entries
}

func fullEntries(version, host string) []Entry {
	if isPre88(version) {
		return []Entry{
			{Name: "operate", URL: fmt.Sprintf("http://%s:8081", host), Notes: "demo/demo"},
			{Name: "tasklist", URL: fmt.Sprintf("http://%s:8082", host), Notes: "demo/demo"},
			{Name: "optimize", URL: fmt.Sprintf("http://%s:8083", host), Notes: "demo/demo"},
			{Name: "identity", URL: fmt.Sprintf("http://%s:8084", host), Notes: "demo/demo"},
			{Name: "connectors", URL: fmt.Sprintf("http://%s:8085", host)},
			{Name: "web-modeler", URL: fmt.Sprintf("http://%s:8070", host), Notes: "demo/demo"},
			{Name: "zeebe-http", URL: fmt.Sprintf("http://%s:8088", host), Notes: "Zeebe gateway HTTP"},
			{Name: "rest", URL: fmt.Sprintf("http://%s:8088", host), Notes: "Desktop Modeler restAddress"},
			{Name: "keycloak", URL: fmt.Sprintf("http://%s:18080/auth/", host), Notes: "admin/admin"},
			{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
			{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)},
		}
	}

	port := orchestrationHostPort(version)
	entries := orchestrationUI(host, port)
	entries = append(entries,
		Entry{Name: "optimize", URL: fmt.Sprintf("http://%s:8083", host), Notes: "demo/demo"},
		Entry{Name: "identity", URL: fmt.Sprintf("http://%s:8084", host), Notes: "demo/demo"},
		Entry{Name: "connectors", URL: fmt.Sprintf("http://%s:8086", host)},
		Entry{Name: "web-modeler", URL: fmt.Sprintf("http://%s:8070", host), Notes: "demo/demo"},
		Entry{Name: "keycloak", URL: fmt.Sprintf("http://%s:18080/auth/", host), Notes: "admin/admin"},
		Entry{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
	)
	// Console exists in 8.8–8.9 full compose; 8.10 full currently ships Hub instead of Console.
	if version == "8.8" || version == "8.9" {
		entries = append(entries, Entry{Name: "console", URL: fmt.Sprintf("http://%s:8087", host), Notes: "demo/demo"})
	}
	// ES is bundled through 8.9; 8.10 full uses our elasticsearch overlay on :9200.
	entries = append(entries, Entry{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)})
	return entries
}

func orchestrationUI(host string, port int) []Entry {
	base := "http://" + host + ":" + strconv.Itoa(port)
	return []Entry{
		{Name: "operate", URL: base + "/operate", Notes: "demo/demo"},
		{Name: "tasklist", URL: base + "/tasklist", Notes: "demo/demo"},
		{Name: "admin", URL: base + "/admin", Notes: "demo/demo"},
		{Name: "rest", URL: base + "/v2", Notes: "Orchestration Cluster REST API"},
		{Name: "orchestration", URL: base, Notes: "Desktop Modeler restAddress (base, no /v2)"},
	}
}

// orchestrationHostPort is the host port mapped to orchestration:8080.
// 8.8 compose still publishes 8088:8080; 8.9+ publish 8080:8080.
func orchestrationHostPort(version string) int {
	if version == "8.8" {
		return 8088
	}
	return 8080
}

func isPre88(version string) bool {
	switch version {
	case "8.5", "8.6", "8.7":
		return true
	default:
		return false
	}
}

// Find returns a named entry or an error.
func Find(cfg config.Config, name string) (Entry, error) {
	for _, e := range List(cfg) {
		if e.Name == name {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("unknown app %q", name)
}

// ModelerRESTBase returns the REST base URL Desktop Modeler expects (no /v2 path).
func ModelerRESTBase(cfg config.Config) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	if isPre88(cfg.Version) {
		return fmt.Sprintf("http://%s:8088", host)
	}
	return fmt.Sprintf("http://%s:%d", host, orchestrationHostPort(cfg.Version))
}
