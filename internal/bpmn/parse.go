package bpmn

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ParseFile reads and parses a BPMN XML file.
func ParseFile(path string) (Model, error) {
	f, err := os.Open(path)
	if err != nil {
		return Model{}, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse parses BPMN XML into a Model.
func Parse(r io.Reader) (Model, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	var doc xmlDoc
	if err := dec.Decode(&doc); err != nil {
		return Model{}, fmt.Errorf("parse bpmn: %w", err)
	}
	return normalize(doc), nil
}

type xmlDoc struct {
	XMLName  xml.Name
	Messages []xmlMessage `xml:"message"`
	Process  []xmlProcess `xml:"process"`
}

type xmlMessage struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

type xmlProcess struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`

	StartEvents  []xmlNode `xml:"startEvent"`
	EndEvents    []xmlNode `xml:"endEvent"`
	ServiceTasks []xmlNode `xml:"serviceTask"`
	UserTasks    []xmlNode `xml:"userTask"`
	ScriptTasks  []xmlNode `xml:"scriptTask"`
	ExclusiveGW  []xmlNode `xml:"exclusiveGateway"`
	ParallelGW   []xmlNode `xml:"parallelGateway"`
	InclusiveGW  []xmlNode `xml:"inclusiveGateway"`
	Intermediate []xmlNode `xml:"intermediateCatchEvent"`
	Boundary     []xmlNode `xml:"boundaryEvent"`
	Flows        []xmlFlow `xml:"sequenceFlow"`
}

type xmlNode struct {
	ID             string     `xml:"id,attr"`
	Name           string     `xml:"name,attr"`
	Default        string     `xml:"default,attr"`
	AttachedToRef  string     `xml:"attachedToRef,attr"`
	TimerEventDef  []xmlTimer `xml:"timerEventDefinition"`
	ErrorEventDef  []xmlError `xml:"errorEventDefinition"`
	MessageEventDef []xmlMsgEv `xml:"messageEventDefinition"`
	ExtensionElements xmlExt  `xml:"extensionElements"`
}

type xmlTimer struct {
	TimeDuration string `xml:"timeDuration"`
	TimeDate     string `xml:"timeDate"`
	TimeCycle    string `xml:"timeCycle"`
}

type xmlError struct {
	ErrorRef string `xml:"errorRef,attr"`
}

type xmlMsgEv struct {
	MessageRef string `xml:"messageRef,attr"`
}

type xmlExt struct {
	TaskDefinition []xmlTaskDef `xml:",any"`
}

type xmlTaskDef struct {
	XMLName  xml.Name
	Type     string `xml:"type,attr"`
	Retries  string `xml:"retries,attr"`
}

type xmlFlow struct {
	ID                  string `xml:"id,attr"`
	Name                string `xml:"name,attr"`
	SourceRef           string `xml:"sourceRef,attr"`
	TargetRef           string `xml:"targetRef,attr"`
	ConditionExpression string `xml:"conditionExpression"`
}

func normalize(doc xmlDoc) Model {
	m := Model{}
	for _, msg := range doc.Messages {
		m.Messages = append(m.Messages, Message{ID: msg.ID, Name: msg.Name})
	}
	if len(doc.Process) == 0 {
		return m
	}
	p := doc.Process[0]
	m.ProcessID = p.ID
	m.Name = p.Name

	add := func(typ string, nodes []xmlNode) {
		for _, n := range nodes {
			el := Element{
				ID:         n.ID,
				Type:       typ,
				Name:       n.Name,
				DefaultFlow: n.Default,
				AttachedTo: n.AttachedToRef,
			}
			for _, t := range n.TimerEventDef {
				el.Timer = firstNonEmpty(t.TimeDuration, t.TimeDate, t.TimeCycle)
				el.EventDefs = append(el.EventDefs, "timer")
			}
			for _, e := range n.ErrorEventDef {
				el.ErrorRef = e.ErrorRef
				el.EventDefs = append(el.EventDefs, "error")
			}
			for _, msg := range n.MessageEventDef {
				el.MessageRef = msg.MessageRef
				el.EventDefs = append(el.EventDefs, "message")
			}
			for _, td := range n.ExtensionElements.TaskDefinition {
				local := td.XMLName.Local
				if local == "taskDefinition" || strings.HasSuffix(local, "taskDefinition") {
					el.JobType = td.Type
					el.RetryCount = td.Retries
				}
			}
			m.Elements = append(m.Elements, el)
		}
	}
	add("startEvent", p.StartEvents)
	add("endEvent", p.EndEvents)
	add("serviceTask", p.ServiceTasks)
	add("userTask", p.UserTasks)
	add("scriptTask", p.ScriptTasks)
	add("exclusiveGateway", p.ExclusiveGW)
	add("parallelGateway", p.ParallelGW)
	add("inclusiveGateway", p.InclusiveGW)
	add("intermediateCatchEvent", p.Intermediate)
	add("boundaryEvent", p.Boundary)

	for _, f := range p.Flows {
		m.Flows = append(m.Flows, Flow{
			ID:        f.ID,
			Name:      f.Name,
			Source:    f.SourceRef,
			Target:    f.TargetRef,
			Condition: strings.TrimSpace(f.ConditionExpression),
		})
	}

	sort.Slice(m.Elements, func(i, j int) bool { return m.Elements[i].ID < m.Elements[j].ID })
	sort.Slice(m.Flows, func(i, j int) bool { return m.Flows[i].ID < m.Flows[j].ID })
	sort.Slice(m.Messages, func(i, j int) bool { return m.Messages[i].ID < m.Messages[j].ID })
	return m
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
