package chroma

import (
	"encoding/xml"
	"fmt"
	"reflect"
)

// Serialisation of Chroma rules to XML. The format is:
//
//     <rules>
//       <state name="$STATE">
//         <rule [pattern="$PATTERN"]>
//           [<$EMITTER ...>]
//           [<$MUTATOR ...>]
//         </rule>
//       </state>
//     </rules>
//
// eg. Include("String") would become:
//
//     <rule>
//       <include state="String" />
//     </rule>
//
//     [null, null, {"kind": "include", "state": "String"}]
//
// eg. Rule{`\d+`, Text, nil} would become:
//
//     <rule pattern="\\d+">
//       <token type="Text"/>
//     </rule>
//
// eg. Rule{`"`, String, Push("String")}
//
//     <rule pattern="\"">
//       <token type="String" />
//       <push state="String" />
//     </rule>
//
// eg. Rule{`(\w+)(\n)`, ByGroups(Keyword, Whitespace), nil},
//
//     <rule pattern="(\\w+)(\\n)">
//       <bygroups token="Keyword" token="Whitespace" />
//       <push state="String" />
//     </rule>

var emitterTemplates = func() map[string]SerialisableEmitter {
	out := map[string]SerialisableEmitter{}
	for _, emitter := range []SerialisableEmitter{
		&byGroupsEmitter{},
		&byGroupNamesEmitter{},
		&usingEmitter{},
		&usingSelfEmitter{},
		TokenType(0),
	} {
		out[emitter.EmitterKind()] = emitter
	}
	return out
}()

var mutatorTemplates = func() map[string]SerialisableMutator {
	out := map[string]SerialisableMutator{}
	for _, mutator := range []SerialisableMutator{
		&includeMutator{},
		&combinedMutator{},
		&multiMutator{},
		&pushMutator{},
		&popMutator{},
	} {
		out[mutator.MutatorKind()] = mutator
	}
	return out
}()

func (r Rule) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "rule"},
	}
	if r.Pattern != "" {
		start.Attr = append(start.Attr, xml.Attr{
			Name:  xml.Name{Local: "pattern"},
			Value: r.Pattern,
		})
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if emitter, ok := r.Type.(SerialisableEmitter); r.Type != nil && ok {
		if err := e.EncodeElement(emitter, xml.StartElement{Name: xml.Name{Local: emitter.EmitterKind()}}); err != nil {
			return err
		}
	}
	if mutator, ok := r.Mutator.(SerialisableMutator); r.Mutator != nil && ok {
		if err := e.EncodeElement(mutator, xml.StartElement{Name: xml.Name{Local: mutator.MutatorKind()}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

func (r *Rule) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "pattern" {
			r.Pattern = attr.Value
			break
		}
	}
	for {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			kind := token.Name.Local
			if emitter, ok := emitterTemplates[kind]; ok {
				if r.Type != nil {
					return fmt.Errorf("duplicate emitter %q", kind)
				}
				value, target := newFromTemplate(emitter)
				if err := d.DecodeElement(target, &token); err != nil {
					return err
				}
				r.Type = value().(SerialisableEmitter)
			} else if mutator, ok := mutatorTemplates[kind]; ok {
				if r.Mutator != nil {
					return fmt.Errorf("duplicate mutator %q", kind)
				}
				value, target := newFromTemplate(mutator)
				if err := d.DecodeElement(target, &token); err != nil {
					return err
				}
				r.Mutator = value().(SerialisableMutator)
			}

		case xml.EndElement:
			return nil
		}
	}
}

type xmlRuleState struct {
	Name  string `xml:"name,attr"`
	Rules []Rule `xml:"rule"`
}

type xmlRules struct {
	States []xmlRuleState `xml:"state"`
}

func (r Rules) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	xr := xmlRules{}
	for state, rules := range r {
		xr.States = append(xr.States, xmlRuleState{
			Name:  state,
			Rules: rules,
		})
	}
	return e.EncodeElement(xr, xml.StartElement{Name: xml.Name{Local: "rules"}})
}

func (r *Rules) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	xr := xmlRules{}
	if err := d.DecodeElement(&xr, &start); err != nil {
		return err
	}
	for _, state := range xr.States {
		(*r)[state.Name] = state.Rules
	}
	return nil
}

type xmlTokenType struct {
	Type string `xml:"type,attr"`
}

func (t *TokenType) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	el := xmlTokenType{}
	if err := d.DecodeElement(&el, &start); err != nil {
		return err
	}
	for tt, text := range _TokenType_map {
		if text == el.Type {
			*t = tt
			return nil
		}
	}
	return fmt.Errorf("unknown TokenType %q", el.Type)
}

func (t TokenType) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "type"}, Value: t.String()})
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// This hijinks is a bit unfortunate but without it we can't deserialise into TokenType.
func newFromTemplate(template interface{}) (value func() interface{}, target interface{}) {
	t := reflect.TypeOf(template)
	if t.Kind() == reflect.Ptr {
		v := reflect.New(t.Elem())
		return v.Interface, v.Interface()
	}
	v := reflect.New(t)
	return func() interface{} { return v.Elem().Interface() }, v.Interface()
}
