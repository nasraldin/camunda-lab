package trace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
)

type traceFactory struct {
	client   *cluster.Client
	resolved env.Resolved
	calls    atomic.Int32
}

func (f *traceFactory) Client(context.Context, string, string) (*cluster.Client, env.Resolved, error) {
	f.calls.Add(1)
	return f.client, f.resolved, nil
}

func testTraceService(t *testing.T, handler http.Handler) (*Service, *traceFactory) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	factory := &traceFactory{
		client: &cluster.Client{BaseURL: server.URL + "/v2", HTTPClient: server.Client(), Kind: cluster.ClientRemote},
		resolved: env.Resolved{
			Profile: env.Profile{
				Name: "prod", Kind: "remote",
				Endpoints: map[string]string{
					"orchestration": server.URL,
					"operate":       "https://operate.example/path",
				},
			},
			Source: env.ProfileSourceProject,
		},
	}
	return NewService(factory), factory
}

func searchEnvelope(items string, total int, cursor any) string {
	body, _ := json.Marshal(map[string]any{
		"items": json.RawMessage(items),
		"page":  map[string]any{"totalItems": total, "endCursor": cursor},
	})
	return string(body)
}

func TestTraceGetDerivesChronologicalTimelineFromCanonicalSearches(t *testing.T) {
	var paths []string
	var filters []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		filters = append(filters, body["filter"].(map[string]any))
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"9007199254740993","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":3,"state":"ACTIVE","hasIncident":true,"tags":[],
			"startDate":"2026-07-21T12:00:00+02:00","endDate":null,"tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		filters = append(filters, body["filter"].(map[string]any))
		_, _ = w.Write([]byte(searchEnvelope(`[
			{
				"elementInstanceKey":"2","processInstanceKey":"9007199254740993","elementId":"pay",
				"elementName":"Payment","type":"SERVICE_TASK","state":"ACTIVE",
				"startDate":"2026-07-21T12:02:00+02:00","endDate":null,"hasIncident":true,
				"incidentKey":"3","processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
			},
			{
				"elementInstanceKey":"1","processInstanceKey":"9007199254740993","elementId":"start",
				"elementName":"OrderCreated","type":"START_EVENT","state":"COMPLETED",
				"startDate":"2026-07-21T12:00:00+02:00","endDate":"2026-07-21T12:00:01+02:00",
				"hasIncident":false,"processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
			}
		]`, 2, nil)))
	})
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		filters = append(filters, body["filter"].(map[string]any))
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"incidentKey":"3","processInstanceKey":"9007199254740993","elementInstanceKey":"2",
			"elementId":"pay","errorType":"JOB_NO_RETRIES",
			"errorMessage":"auth failed bearer super-secret token","state":"ACTIVE",
			"creationTime":"2026-07-21T12:03:00+02:00","processDefinitionId":"orders",
			"processDefinitionKey":"8","tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		filters = append(filters, body["filter"].(map[string]any))
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"jobKey":"9007199254740994","processInstanceKey":"9007199254740993","elementInstanceKey":"2",
			"processDefinitionKey":"8","processDefinitionId":"orders","elementId":"pay",
			"type":"payment","state":"FAILED","worker":"worker-a","retries":0,
			"creationTime":"2026-07-21T12:02:30+02:00","lastUpdateTime":"2026-07-21T12:03:00+02:00",
			"errorMessage":"auth failed bearer super-secret token","customHeaders":{},
			"hasFailedWithRetriesLeft":false,"kind":"BPMN_ELEMENT","listenerEventType":"UNSPECIFIED",
			"tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/process-instances/", func(http.ResponseWriter, *http.Request) {
		t.Fatal("ad hoc process-instance GET must not be used")
	})

	service, factory := testTraceService(t, mux)
	got, err := service.Get(context.Background(), Request{
		Environment: "prod", ProjectRoot: "/project", ProcessInstanceKey: "9007199254740993",
	})
	if err != nil {
		t.Fatal(err)
	}
	if factory.calls.Load() != 1 || got.Environment.Profile.Name != "prod" ||
		got.Source.Environment != "prod" || got.Source.Type != "remote" ||
		got.InstanceKey != "9007199254740993" || got.State != "ACTIVE" ||
		!got.Complete || got.Partial || got.Status != StatusCompleted ||
		!strings.Contains(got.OperateURL, "/processes/9007199254740993") {
		t.Fatalf("unexpected timeline meta: %+v", got)
	}
	if len(got.Events) < 5 {
		t.Fatalf("expected process/element/incident/job events, got %#v", got.Events)
	}
	for i := 1; i < len(got.Events); i++ {
		if got.Events[i-1].Timestamp > got.Events[i].Timestamp {
			t.Fatalf("events not chronological: %#v", got.Events)
		}
	}
	var sawProcess, sawElement, sawIncident, sawJob bool
	for _, event := range got.Events {
		switch event.Kind {
		case EventProcess:
			sawProcess = true
			if event.Key != "9007199254740993" {
				t.Fatalf("process key mutated: %#v", event)
			}
		case EventElement:
			sawElement = true
		case EventIncident:
			sawIncident = true
			if strings.Contains(event.Detail, "super-secret") {
				t.Fatalf("incident detail not redacted: %#v", event)
			}
		case EventJob:
			sawJob = true
			if event.Key != "9007199254740994" || strings.Contains(event.Detail, "super-secret") {
				t.Fatalf("job event invalid: %#v", event)
			}
		}
	}
	if !sawProcess || !sawElement || !sawIncident || !sawJob {
		t.Fatalf("missing event kinds: %#v", got.Events)
	}
	for _, filter := range filters {
		if filter["processInstanceKey"] != "9007199254740993" {
			t.Fatalf("unscoped or wrong filter: %#v", filter)
		}
	}
	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "/v2/process-instances/search") ||
		!strings.Contains(joined, "/v2/element-instances/search") ||
		!strings.Contains(joined, "/v2/incidents/search") ||
		!strings.Contains(joined, "/v2/jobs/search") {
		t.Fatalf("paths = %v", paths)
	}
	text := FormatText(got)
	ascii := RenderASCII(got)
	encoded, err := FormatJSON(got)
	if err != nil || !strings.Contains(text, "9007199254740993") ||
		!strings.Contains(ascii, "INCIDENT") || !strings.Contains(ascii, "↓") ||
		!strings.Contains(string(encoded), `"events": [`) ||
		!strings.Contains(string(encoded), `"warnings": []`) ||
		strings.Contains(string(encoded), `"events": null`) {
		t.Fatalf("formats text=%q ascii=%q json=%s err=%v", text, ascii, encoded, err)
	}
}

func TestTraceGetReturnsTypedNotFoundForMissingRoot(t *testing.T) {
	service, _ := testTraceService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/process-instances/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	}))
	_, err := service.Get(context.Background(), Request{ProcessInstanceKey: "99"})
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || notFound.Key != "99" {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestTraceGetRejectsInvalidKeyWithoutClusterCall(t *testing.T) {
	var requests atomic.Int32
	service, _ := testTraceService(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	for _, key := range []string{"", " 99 ", "0", "01", "abc", "-1"} {
		if _, err := service.Get(context.Background(), Request{ProcessInstanceKey: key}); err == nil {
			t.Fatalf("Get(%q) succeeded", key)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid keys reached cluster %d times", requests.Load())
	}
}

func TestTraceGetWarnsOnOptionalChildPartialAndKeepsPartialStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"elementInstanceKey":"1","processInstanceKey":"55","elementId":"start",
			"elementName":"Start","type":"START_EVENT","state":"COMPLETED",
			"startDate":"2026-07-21T12:00:00Z","endDate":"2026-07-21T12:00:01Z",
			"hasIncident":false,"processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
		}]`, 2, "next")))
	})
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"optional job index unavailable"}`, http.StatusServiceUnavailable)
	})
	service, _ := testTraceService(t, mux)
	got, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55", MaxItems: 1, PageSize: 1, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || !got.Partial || got.Status != StatusPartial || len(got.Warnings) == 0 {
		t.Fatalf("expected partial child warnings: %+v", got)
	}
	capabilities := map[string]bool{}
	for _, warning := range got.Warnings {
		capabilities[warning.Capability] = true
	}
	if !capabilities["element-instances"] || !capabilities["jobs"] {
		t.Fatalf("warnings = %#v", got.Warnings)
	}
}

func TestTraceGetRejectsExactProcessSearchAnomaly(t *testing.T) {
	service, _ := testTraceService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
		}]`, 2, "next")))
	}))
	_, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55"})
	if err == nil || errors.As(err, new(*NotFoundError)) {
		t.Fatalf("expected anomaly error, got %v", err)
	}
}

func TestTraceGetPreservesMalformedAndStatusErrors(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		service, _ := testTraceService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"items":[{"processInstanceKey":"55","startDate":"not-a-timestamp","state":"ACTIVE","processDefinitionId":"orders","processDefinitionKey":"8","processDefinitionVersion":1,"hasIncident":false,"tags":[],"tenantId":"<default>"}],"page":{"totalItems":1}}`))
		}))
		_, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55"})
		if err == nil {
			t.Fatal("expected decode failure")
		}
	})
	t.Run("status", func(t *testing.T) {
		service, _ := testTraceService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"boom token=super-secret"}`, http.StatusBadGateway)
		}))
		_, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55"})
		var apiErr *cluster.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway {
			t.Fatalf("error = %T %v", err, err)
		}
		if strings.Contains(err.Error(), "super-secret") {
			t.Fatalf("secret leaked: %v", err)
		}
	})
}

func TestTraceGetDoesNotMutate(t *testing.T) {
	var mutations atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			mutations.Add(1)
		}
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"COMPLETED","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","endDate":"2026-07-21T12:05:00Z","tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			mutations.Add(1)
		}
		if strings.Contains(r.URL.Path, "resolution") || strings.Contains(r.URL.Path, "cancellation") {
			mutations.Add(1)
		}
		http.NotFound(w, r)
	})
	service, _ := testTraceService(t, mux)
	if _, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55"}); err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations observed: %d", mutations.Load())
	}
}

func TestTraceFollowBoundsCancelIdleAndChangedOnly(t *testing.T) {
	var polls atomic.Int32
	states := []string{"ACTIVE", "ACTIVE", "ACTIVE", "COMPLETED"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		index := int(polls.Add(1) - 1)
		if index >= len(states) {
			index = len(states) - 1
		}
		state := states[index]
		end := "null"
		if state == "COMPLETED" {
			end = `"2026-07-21T12:05:00Z"`
		}
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"`+state+`","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","endDate":`+end+`,"tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		count := polls.Load()
		items := `[{
			"elementInstanceKey":"1","processInstanceKey":"55","elementId":"start",
			"elementName":"Start","type":"START_EVENT","state":"COMPLETED",
			"startDate":"2026-07-21T12:00:00Z","endDate":"2026-07-21T12:00:01Z",
			"hasIncident":false,"processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
		}]`
		if count >= 3 {
			items = `[{
				"elementInstanceKey":"1","processInstanceKey":"55","elementId":"start",
				"elementName":"Start","type":"START_EVENT","state":"COMPLETED",
				"startDate":"2026-07-21T12:00:00Z","endDate":"2026-07-21T12:00:01Z",
				"hasIncident":false,"processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
			},{
				"elementInstanceKey":"2","processInstanceKey":"55","elementId":"end",
				"elementName":"Done","type":"END_EVENT","state":"COMPLETED",
				"startDate":"2026-07-21T12:04:00Z","endDate":"2026-07-21T12:05:00Z",
				"hasIncident":false,"processDefinitionKey":"8","processDefinitionId":"orders","tenantId":"<default>"
			}]`
		}
		_, _ = w.Write([]byte(searchEnvelope(items, 1, nil)))
	})
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	service, _ := testTraceService(t, mux)

	var slept []time.Duration
	oldWait, oldNow := Wait, Now
	defer func() { Wait = oldWait; Now = oldNow }()
	current := time.Unix(0, 0).UTC()
	Now = func() time.Time { return current }
	Wait = func(ctx context.Context, d time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		slept = append(slept, d)
		current = current.Add(d)
		return nil
	}

	var emissions []Timeline
	err := service.Follow(context.Background(), Request{
		ProcessInstanceKey: "55",
		Timeout:            10 * time.Second,
		MaxEvents:          10,
		IdleStop:           0,
	}, time.Second, func(tl Timeline) error {
		emissions = append(emissions, tl)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(emissions) < 2 || emissions[len(emissions)-1].State != "COMPLETED" {
		t.Fatalf("emissions=%#v", emissions)
	}
	if len(slept) == 0 || slept[0] != time.Second {
		t.Fatalf("expected interval sleeps, got %#v", slept)
	}
	// Unchanged middle poll must not emit.
	changedOnly := 0
	for i := 1; i < len(emissions); i++ {
		if timelineFingerprint(emissions[i]) != timelineFingerprint(emissions[i-1]) {
			changedOnly++
		}
	}
	if changedOnly != len(emissions)-1 {
		t.Fatalf("duplicate emissions: %#v", emissions)
	}

	t.Run("cancel", func(t *testing.T) {
		polls.Store(0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := service.Follow(ctx, Request{ProcessInstanceKey: "55", Timeout: time.Minute}, time.Second, func(Timeline) error {
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		polls.Store(0)
		states = []string{"ACTIVE", "ACTIVE", "ACTIVE", "ACTIVE", "ACTIVE"}
		current = time.Unix(0, 0).UTC()
		slept = nil
		err := service.Follow(context.Background(), Request{
			ProcessInstanceKey: "55", Timeout: 2 * time.Second, MaxEvents: 100,
		}, time.Second, func(Timeline) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("idle stop", func(t *testing.T) {
		polls.Store(0)
		states = []string{"ACTIVE", "ACTIVE", "ACTIVE", "ACTIVE"}
		current = time.Unix(0, 0).UTC()
		err := service.Follow(context.Background(), Request{
			ProcessInstanceKey: "55", Timeout: time.Minute, IdleStop: 2 * time.Second, MaxEvents: 100,
		}, time.Second, func(Timeline) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("max events", func(t *testing.T) {
		polls.Store(0)
		states = []string{"ACTIVE", "ACTIVE", "ACTIVE", "ACTIVE"}
		current = time.Unix(0, 0).UTC()
		var count int
		err := service.Follow(context.Background(), Request{
			ProcessInstanceKey: "55", Timeout: time.Minute, MaxEvents: 1,
		}, time.Second, func(Timeline) error {
			count++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("count = %d", count)
		}
	})
}

func TestTraceOperateLinkValidatesAndEscapes(t *testing.T) {
	link, err := OperateLink("https://operate.example/base", "9007199254740993")
	if err != nil || !strings.Contains(link, "/processes/9007199254740993") {
		t.Fatalf("link=%q err=%v", link, err)
	}
	if _, err := OperateLink("operate.example", "1"); err == nil {
		t.Fatal("expected invalid base URL error")
	}
	if _, err := OperateLink("https://user:pass@operate.example", "1"); err == nil {
		t.Fatal("expected userinfo rejection")
	}
}

func TestFollowOnce(t *testing.T) {
	a := FromActivities("1", "ACTIVE", []Step{{Name: "A"}})
	b := FromActivities("1", "ACTIVE", []Step{{Name: "A"}, {Name: "B"}})
	_, changed := FollowOnce(a, b)
	if !changed {
		t.Fatal("expected change")
	}
	_, changed = FollowOnce(b, b)
	if changed {
		t.Fatal("expected no change")
	}
}

func TestFollowCancelsMidWait(t *testing.T) {
	var mutations atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			mutations.Add(1)
		}
		_, _ = w.Write([]byte(searchEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","endDate":null,"tenantId":"<default>"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			mutations.Add(1)
		}
	})
	service, _ := testTraceService(t, mux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oldWait, oldNow := Wait, Now
	defer func() { Wait = oldWait; Now = oldNow }()
	current := time.Unix(0, 0).UTC()
	Now = func() time.Time { return current }

	waitStarted := make(chan struct{})
	var waitCount atomic.Int32
	Wait = func(waitCtx context.Context, d time.Duration) error {
		if d < minPollInterval {
			t.Fatalf("busy-loop wait duration: %s", d)
		}
		if waitCount.Add(1) == 1 {
			close(waitStarted)
		}
		select {
		case <-waitCtx.Done():
			return waitCtx.Err()
		case <-time.After(5 * time.Second):
			t.Fatal("wait did not observe cancellation")
			return nil
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- service.Follow(ctx, Request{
			ProcessInstanceKey: "55",
			Timeout:            time.Minute,
			MaxEvents:          10,
		}, time.Second, func(Timeline) error { return nil })
	}()

	select {
	case <-waitStarted:
	case err := <-done:
		t.Fatalf("follow returned before wait started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for follow wait to start")
	}
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations observed: %d", mutations.Load())
	}
	if waitCount.Load() < 1 {
		t.Fatal("expected at least one mid-follow wait")
	}
}

func TestTraceJobTimestampFallbackUsesEndTimeDeadlineWithEndPhase(t *testing.T) {
	cases := []struct {
		name      string
		jobFields string
		wantTS    string
		wantPhase string
	}{
		{
			name:      "endTime when creation and lastUpdate null",
			jobFields: `"creationTime":null,"lastUpdateTime":null,"endTime":"2026-07-21T12:03:00+02:00","deadline":"2026-07-21T12:01:00+02:00"`,
			wantTS:    "2026-07-21T10:03:00Z",
			wantPhase: "end",
		},
		{
			name:      "deadline when only deadline remains",
			jobFields: `"creationTime":null,"lastUpdateTime":null,"endTime":null,"deadline":"2026-07-21T12:01:00+02:00"`,
			wantTS:    "2026-07-21T10:01:00Z",
			wantPhase: "end",
		},
		{
			name:      "lastUpdate preferred over endTime",
			jobFields: `"creationTime":null,"lastUpdateTime":"2026-07-21T12:02:45+02:00","endTime":"2026-07-21T12:03:00+02:00","deadline":"2026-07-21T12:01:00+02:00"`,
			wantTS:    "2026-07-21T10:02:45Z",
			wantPhase: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(searchEnvelope(`[{
					"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
					"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
					"startDate":"2026-07-21T12:00:00Z","endDate":null,"tenantId":"<default>"
				}]`, 1, nil)))
			})
			mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
			})
			mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(searchEnvelope(`[]`, 0, nil)))
			})
			mux.HandleFunc("/v2/jobs/search", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(searchEnvelope(`[{
					"jobKey":"9007199254740994","processInstanceKey":"55","elementInstanceKey":"2",
					"processDefinitionKey":"8","processDefinitionId":"orders","elementId":"ship",
					"type":"shipping","state":"COMPLETED","worker":"worker-a","retries":0,
					"errorMessage":null,"customHeaders":{},"hasFailedWithRetriesLeft":false,
					"kind":"BPMN_ELEMENT","listenerEventType":"UNSPECIFIED","tenantId":"<default>",
					`+tc.jobFields+`
				}]`, 1, nil)))
			})
			service, _ := testTraceService(t, mux)
			got, err := service.Get(context.Background(), Request{ProcessInstanceKey: "55"})
			if err != nil {
				t.Fatal(err)
			}
			var jobEvent *Event
			for i := range got.Events {
				if got.Events[i].Kind == EventJob {
					jobEvent = &got.Events[i]
					break
				}
			}
			if jobEvent == nil {
				t.Fatalf("job event dropped: %#v", got.Events)
			}
			if jobEvent.Timestamp != tc.wantTS || jobEvent.Phase != tc.wantPhase || jobEvent.Key != "9007199254740994" {
				t.Fatalf("job event = %#v, want ts=%q phase=%q", jobEvent, tc.wantTS, tc.wantPhase)
			}
		})
	}
}
