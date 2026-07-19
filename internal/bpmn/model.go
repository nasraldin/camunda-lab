package bpmn

// Model is a normalized process definition IR.
type Model struct {
	ProcessID string
	Name      string
	Elements  []Element
	Flows     []Flow
	Messages  []Message
}

// Element is a flow node (task, event, gateway, …).
type Element struct {
	ID          string
	Type        string // startEvent, endEvent, serviceTask, userTask, scriptTask, exclusiveGateway, parallelGateway, inclusiveGateway, intermediateCatchEvent, boundaryEvent, …
	Name        string
	DefaultFlow string // exclusive gateway default
	Timer       string // timer duration/date/cycle if present
	RetryCount  string // zeebe:taskDefinition retries or similar
	ErrorRef    string
	MessageRef  string
	JobType     string // zeebe task type
	AttachedTo  string // boundary event
	EventDefs   []string
}

// Flow is a sequence flow.
type Flow struct {
	ID        string
	Name      string
	Source    string
	Target    string
	Condition string
}

// Message is a BPMN message definition.
type Message struct {
	ID   string
	Name string
}

// ElementByID returns an element or nil.
func (m Model) ElementByID(id string) *Element {
	for i := range m.Elements {
		if m.Elements[i].ID == id {
			return &m.Elements[i]
		}
	}
	return nil
}

// ServiceTasks returns service/script-style external tasks.
func (m Model) ServiceTasks() []Element {
	var out []Element
	for _, e := range m.Elements {
		if e.Type == "serviceTask" || e.Type == "scriptTask" {
			out = append(out, e)
		}
	}
	return out
}
