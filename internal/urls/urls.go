package urls

import (
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/config"
)

type Entry struct {
	Name  string
	URL   string
	Notes string
}

func List(cfg config.Config) []Entry {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	switch cfg.Profile {
	case "modeler":
		return []Entry{
			{Name: "web-modeler", URL: fmt.Sprintf("http://%s:8070", host)},
		}
	case "full":
		entries := []Entry{
			{Name: "operate", URL: fmt.Sprintf("http://%s:8088/operate", host), Notes: "demo/demo"},
			{Name: "tasklist", URL: fmt.Sprintf("http://%s:8088/tasklist", host), Notes: "demo/demo"},
			{Name: "console", URL: fmt.Sprintf("http://%s:8087", host), Notes: "demo/demo"},
			{Name: "optimize", URL: fmt.Sprintf("http://%s:8083", host), Notes: "demo/demo"},
			{Name: "identity", URL: fmt.Sprintf("http://%s:8084", host), Notes: "demo/demo"},
			{Name: "web-modeler", URL: fmt.Sprintf("http://%s:8070", host), Notes: "demo/demo"},
			{Name: "keycloak", URL: fmt.Sprintf("http://%s:18080/auth/", host), Notes: "admin/admin"},
			{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
			{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)},
		}
		if cfg.Version == "8.7" {
			entries[0].URL = fmt.Sprintf("http://%s:8081", host) // operate typical 8.7
			entries[1].URL = fmt.Sprintf("http://%s:8082", host) // tasklist typical 8.7
		}
		return entries
	default: // light
		if cfg.Version == "8.7" {
			return []Entry{
				{Name: "operate", URL: fmt.Sprintf("http://%s:8081", host), Notes: "demo/demo"},
				{Name: "tasklist", URL: fmt.Sprintf("http://%s:8082", host), Notes: "demo/demo"},
				{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
				{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)},
			}
		}
		return []Entry{
			{Name: "operate", URL: fmt.Sprintf("http://%s:8080/operate", host), Notes: "demo/demo"},
			{Name: "tasklist", URL: fmt.Sprintf("http://%s:8080/tasklist", host), Notes: "demo/demo"},
			{Name: "admin", URL: fmt.Sprintf("http://%s:8080/admin", host), Notes: "demo/demo"},
			{Name: "rest", URL: fmt.Sprintf("http://%s:8080/v2", host)},
			{Name: "grpc", URL: fmt.Sprintf("%s:26500", host)},
			{Name: "elasticsearch", URL: fmt.Sprintf("http://%s:9200", host)},
		}
	}
}

func Find(cfg config.Config, name string) (Entry, error) {
	for _, e := range List(cfg) {
		if e.Name == name {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("unknown app %q", name)
}
