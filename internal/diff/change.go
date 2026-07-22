package diff

// ChangeKind is retained as a source-compatible name for the public string contract.
type ChangeKind = string

const (
	ProcessAdded   ChangeKind = "process_added"
	ProcessRemoved ChangeKind = "process_removed"
	ElementAdded   ChangeKind = "element_added"
	ElementRemoved ChangeKind = "element_removed"
	FieldChanged   ChangeKind = "field_changed"
	FlowAdded      ChangeKind = "flow_added"
	FlowRemoved    ChangeKind = "flow_removed"
	MessageAdded   ChangeKind = "message_added"
	MessageRemoved ChangeKind = "message_removed"
	ErrorAdded     ChangeKind = "error_added"
	ErrorRemoved   ChangeKind = "error_removed"

	// AttrChanged and FlowChanged are retained for source compatibility.
	AttrChanged ChangeKind = FieldChanged
	FlowChanged ChangeKind = FieldChanged
)

// Change is a presentation-independent semantic difference.
type Change struct {
	Kind        ChangeKind `json:"kind"`
	ProcessID   string     `json:"processId,omitempty"`
	ElementID   string     `json:"elementId,omitempty"`
	ElementType string     `json:"elementType,omitempty"`
	Field       string     `json:"field,omitempty"`
	Before      string     `json:"before,omitempty"`
	After       string     `json:"after,omitempty"`
	Summary     string     `json:"summary"`

	// ID is the legacy element/message identifier exposed by the D1 contract.
	// New callers should use ElementID.
	ID string `json:"-"`
}
