package nrtm

import (
	"strings"
)

type RPSLObject struct {
	Type       string
	Attributes map[string]string
	RawText    string
}

type AutNum struct {
	ASN     string
	AsName  string
	Descr   string
	Country string
	Source  string
	Org     string
	OrgName string
	Status  string
	Created string
}

func ParseRPSLObject(text string) *RPSLObject {
	if text == "" {
		return nil
	}

	obj := &RPSLObject{
		Attributes: make(map[string]string),
		RawText:    text,
	}

	lines := strings.Split(text, "\n")
	var currentAttr string
	var currentValue strings.Builder

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t' || line[0] == '+') {
			if currentAttr != "" {
				currentValue.WriteString(" ")
				currentValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		if currentAttr != "" {
			if _, exists := obj.Attributes[currentAttr]; !exists {
				obj.Attributes[currentAttr] = currentValue.String()
			}
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		currentAttr = strings.TrimSpace(line[:colonIdx])
		currentValue.Reset()
		currentValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))

		if obj.Type == "" {
			obj.Type = currentAttr
		}
	}

	if currentAttr != "" {
		if _, exists := obj.Attributes[currentAttr]; !exists {
			obj.Attributes[currentAttr] = currentValue.String()
		}
	}

	if obj.Type == "" {
		return nil
	}

	return obj
}

func (o *RPSLObject) IsAutNum() bool {
	return o.Type == "aut-num"
}

func (o *RPSLObject) ToAutNum() *AutNum {
	if !o.IsAutNum() {
		return nil
	}

	return &AutNum{
		ASN:     o.Attributes["aut-num"],
		AsName:  o.Attributes["as-name"],
		Descr:   o.Attributes["descr"],
		Country: o.Attributes["country"],
		Source:  o.Attributes["source"],
		Org:     o.Attributes["org"],
		OrgName: o.Attributes["org-name"],
		Status:  o.Attributes["status"],
		Created: o.Attributes["created"],
	}
}

func (o *RPSLObject) GetAttribute(name string) string {
	return o.Attributes[name]
}
