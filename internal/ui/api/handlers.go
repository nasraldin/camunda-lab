package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/laberrors"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/smoke"
	"github.com/nasraldin/camunda-lab/internal/tools"
	"github.com/nasraldin/camunda-lab/internal/ui/sso"
	"github.com/nasraldin/camunda-lab/internal/update"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

// Register mounts /api/v1 routes on mux.
func Register(mux *http.ServeMux, cliVersion string) {
	h := &handler{cliVersion: cliVersion, lab: lab.New()}
	mux.HandleFunc("GET /api/v1/overview", h.overview)
	mux.HandleFunc("POST /api/v1/install", h.install)
	mux.HandleFunc("POST /api/v1/recover", h.recover)
	mux.HandleFunc("POST /api/v1/up", h.up)
	mux.HandleFunc("POST /api/v1/down", h.down)
	mux.HandleFunc("POST /api/v1/restart", h.restart)
	mux.HandleFunc("POST /api/v1/switch", h.switchVersion)
	mux.HandleFunc("POST /api/v1/profile", h.setProfile)
	mux.HandleFunc("POST /api/v1/resources", h.setResources)
	mux.HandleFunc("GET /api/v1/urls", h.listURLs)
	mux.HandleFunc("GET /api/v1/probe", h.probe)
	mux.HandleFunc("GET /api/v1/sso/open", h.ssoOpen)
	mux.HandleFunc("GET /api/v1/doctor", h.runDoctor)
	mux.HandleFunc("GET /api/v1/smoke", h.runSmoke)
	mux.HandleFunc("GET /api/v1/containers", h.containers)
	mux.HandleFunc("POST /api/v1/containers/{service}/restart", h.restartService)
	mux.HandleFunc("GET /api/v1/logs/{service}", h.logs)
	mux.HandleFunc("GET /api/v1/ai/status", h.aiStatus)
	mux.HandleFunc("POST /api/v1/ai/enable", h.aiEnable)
	mux.HandleFunc("POST /api/v1/ai/disable", h.aiDisable)
	mux.HandleFunc("GET /api/v1/ai/config", h.aiConfig)
	mux.HandleFunc("GET /api/v1/tools/c8ctl/status", h.c8ctlStatus)
	mux.HandleFunc("POST /api/v1/tools/c8ctl/install", h.c8ctlInstall)
	mux.HandleFunc("POST /api/v1/tools/modeler/profile", h.modelerProfile)
	mux.HandleFunc("POST /api/v1/tools/deploy", h.deploy)
	mux.HandleFunc("POST /api/v1/nuke", h.nuke)
	mux.HandleFunc("GET /api/v1/update", h.updateCheck)
	mux.HandleFunc("POST /api/v1/update", h.updateApply)
}

type handler struct {
	cliVersion string
	lab        *lab.Lab
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	err = laberrors.Wrap(err)
	payload := map[string]any{"error": err.Error()}
	if u, ok := laberrors.AsUser(err); ok {
		payload["error"] = u.Message
		if u.Hint != "" {
			payload["hint"] = u.Hint
		}
		if u.Code != "" {
			payload["code"] = u.Code
		}
		if u.Recoverable {
			payload["recoverable"] = true
		}
	}
	writeJSON(w, status, payload)
}

func (h *handler) recover(w http.ResponseWriter, r *http.Request) {
	if err := h.lab.Recover(r.Context()); err != nil {
		writeErr(w, http.StatusBadRequest, laberrors.Wrap(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func (h *handler) overview(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	configured := err == nil && cfg.Version != ""
	if err != nil && !os.IsNotExist(err) {
		// Load returns defaults on missing file
	}
	out := map[string]any{
		"cliVersion": h.cliVersion,
		"labHome":    paths.Home(),
		"configured": configured && cfg.Version != "",
		"config": map[string]any{
			"version":   cfg.Version,
			"profile":   cfg.Profile,
			"resources": cfg.Resources,
			"host":      cfg.Host,
			"project":   cfg.ComposeProject,
			"aiEnabled": cfg.AI.Enabled,
		},
		"supportedVersions": versions.Supported,
		"uiHint":            "http://localhost:9090",
	}
	if configured && cfg.Version != "" {
		containers, cErr := h.lab.ListContainers(r.Context())
		if cErr == nil {
			running := 0
			for _, c := range containers {
				if c.State == "running" {
					running++
				}
			}
			out["containers"] = containers
			out["running"] = running
			out["total"] = len(containers)
		} else {
			out["containersError"] = cErr.Error()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type installBody struct {
	Version       string `json:"version"`
	Profile       string `json:"profile"`
	Resources     string `json:"resources"`
	AI            bool   `json:"ai"`
	OpenAIKey     string `json:"openaiKey"`
	AnthropicKey  string `json:"anthropicKey"`
	OpenAIBaseURL string `json:"openaiBaseUrl"`
}

func (h *handler) install(w http.ResponseWriter, r *http.Request) {
	var body installBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	opts := lab.InstallOpts{
		Version:   body.Version,
		Profile:   body.Profile,
		Resources: body.Resources,
		Yes:       true,
		AI:        body.AI,
		AISecrets: ai.Secrets{
			OpenAIKey:     body.OpenAIKey,
			AnthropicKey:  body.AnthropicKey,
			OpenAIBaseURL: body.OpenAIBaseURL,
		},
	}
	if err := h.lab.Install(r.Context(), opts); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) up(w http.ResponseWriter, r *http.Request) {
	if err := h.lab.Up(r.Context()); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) down(w http.ResponseWriter, r *http.Request) {
	if err := h.lab.Down(r.Context(), false); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) restart(w http.ResponseWriter, r *http.Request) {
	if err := h.lab.Down(r.Context(), false); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := h.lab.Up(r.Context()); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type switchBody struct {
	Version       string `json:"version"`
	Wipe          bool   `json:"wipe"`
	AI            bool   `json:"ai"`
	OpenAIKey     string `json:"openaiKey"`
	AnthropicKey  string `json:"anthropicKey"`
	OpenAIBaseURL string `json:"openaiBaseUrl"`
}

func (h *handler) switchVersion(w http.ResponseWriter, r *http.Request) {
	var body switchBody
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.AI {
		cfg, err := config.Load()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		s := ai.Secrets{OpenAIKey: body.OpenAIKey, AnthropicKey: body.AnthropicKey, OpenAIBaseURL: body.OpenAIBaseURL}
		if !s.Configured() {
			existing, _ := ai.LoadSecrets()
			s = existing
		}
		if err := ai.ValidateForEnable(body.Version, cfg.Profile, s); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := h.lab.Switch(r.Context(), body.Version, body.Wipe); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := h.lab.EnableAI(r.Context(), s); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err := h.lab.Switch(r.Context(), body.Version, body.Wipe); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) setProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profile string `json:"profile"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := h.lab.SetProfile(r.Context(), body.Profile); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) setResources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Resources string `json:"resources"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := h.lab.SetResources(r.Context(), body.Resources); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) listURLs(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	entries := urls.List(cfg)
	writeJSON(w, http.StatusOK, map[string]any{"urls": entries})
}

func (h *handler) probe(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing name query parameter"))
		return
	}
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	entry, err := urls.Find(cfg, name)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	res := urls.ProbeEntry(ctx, entry, 5*time.Second)
	writeJSON(w, http.StatusOK, res)
}

// ssoOpen warms Keycloak SSO cookies in the browser, then redirects to the app.
// Requires Lab UI on http://localhost (same cookie host as Camunda).
func (h *handler) ssoOpen(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if target == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing url query parameter"))
		return
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid url"))
		return
	}
	host := strings.ToLower(u.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("refusing non-loopback open target"))
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	kc, err := urls.Find(cfg, "keycloak")
	if err != nil || kc.URL == "" {
		// No Keycloak — just redirect (light profile).
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	cookies, err := sso.SessionCookies(ctx, kc.URL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	sso.WriteCookies(w, cookies)
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *handler) runDoctor(w http.ResponseWriter, r *http.Request) {
	rep := doctor.Run(false)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     rep.OK,
		"report": rep.Format(),
	})
}

func (h *handler) runSmoke(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res := smoke.Probe(ctx, cfg)
	writeJSON(w, http.StatusOK, res)
}

func (h *handler) containers(w http.ResponseWriter, r *http.Request) {
	list, err := h.lab.ListContainers(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": list})
}

func (h *handler) restartService(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	if err := h.lab.RestartService(r.Context(), service); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) logs(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}

	sw := &sseWriter{w: w, f: flusher}
	err := h.lab.StreamLogs(r.Context(), service, follow, sw)
	if err != nil && r.Context().Err() == nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
	}
}

type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
	b []byte
}

func (s *sseWriter) Write(p []byte) (int, error) {
	s.b = append(s.b, p...)
	for {
		i := -1
		for j, c := range s.b {
			if c == '\n' {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		line := string(s.b[:i])
		s.b = s.b[i+1:]
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		fmt.Fprintf(s.w, "data: %s\n\n", line)
		s.f.Flush()
	}
	return len(p), nil
}

func (h *handler) aiStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s, _ := ai.LoadSecrets()
	supportErr := versions.SupportsAIFeature(cfg.Version, cfg.Profile)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       cfg.AI.Enabled,
		"openaiKey":     ai.Mask(s.OpenAIKey),
		"anthropicKey":  ai.Mask(s.AnthropicKey),
		"openaiBaseUrl": s.OpenAIBaseURL,
		"supported":     supportErr == nil,
		"supportError":  errString(supportErr),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *handler) aiEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OpenAIKey     string `json:"openaiKey"`
		AnthropicKey  string `json:"anthropicKey"`
		OpenAIBaseURL string `json:"openaiBaseUrl"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s := ai.Secrets{OpenAIKey: body.OpenAIKey, AnthropicKey: body.AnthropicKey, OpenAIBaseURL: body.OpenAIBaseURL}
	if !s.Configured() {
		existing, _ := ai.LoadSecrets()
		if body.OpenAIKey == "" {
			s.OpenAIKey = existing.OpenAIKey
		}
		if body.AnthropicKey == "" {
			s.AnthropicKey = existing.AnthropicKey
		}
		if body.OpenAIBaseURL == "" {
			s.OpenAIBaseURL = existing.OpenAIBaseURL
		}
	}
	if err := h.lab.EnableAI(r.Context(), s); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) aiDisable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WipeSecrets bool `json:"wipeSecrets"`
	}
	_ = decodeJSON(r, &body)
	if err := h.lab.DisableAI(r.Context(), body.WipeSecrets); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) aiConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := ai.MCPClientConfig(cfg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"config": out})
}

func (h *handler) c8ctlStatus(w http.ResponseWriter, r *http.Request) {
	installed, path, err := tools.C8ctlStatus()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installed": installed, "path": path})
}

func (h *handler) c8ctlInstall(w http.ResponseWriter, r *http.Request) {
	if err := tools.C8ctlInstall(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) modelerProfile(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rest := urls.ModelerRESTBase(cfg)
	grpc := ""
	for _, e := range urls.List(cfg) {
		if e.Name == "grpc" {
			grpc = e.URL
			break
		}
	}
	path, err := tools.WriteModelerProfile("camunda-lab", rest, grpc)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "rest": rest, "grpc": grpc})
}

func (h *handler) deploy(w http.ResponseWriter, r *http.Request) {
	installed, bin, err := tools.C8ctlStatus()
	if err != nil || !installed {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("c8ctl not installed — use Tools to install"))
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("file required"))
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "camunda-lab-deploy-*"+filepath.Ext(hdr.Filename))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = tmp.Close()

	cmd := exec.Command(bin, "deploy", tmp.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(string(out))))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "output": string(out)})
}

func (h *handler) nuke(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Confirm != "DELETE" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf(`type confirm: "DELETE"`))
		return
	}
	if err := h.lab.Nuke(r.Context(), true); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) updateCheck(w http.ResponseWriter, r *http.Request) {
	info := update.Check(r.Context(), h.cliVersion)
	writeJSON(w, http.StatusOK, info)
}

func (h *handler) updateApply(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	res, err := update.Apply(ctx, h.cliVersion)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":          false,
			"error":       err.Error(),
			"channel":     res.Channel,
			"output":      res.Output,
			"restartHint": res.RestartHint,
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
