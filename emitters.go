package chroma

// An Emitter takes group matches and returns tokens.
type Emitter interface {
	// Emit tokens for the given regex groups.
	Emit(groups []string, state *LexerState) Iterator
}

// SerialisableEmitter is an Emitter that can be serialised and deserialised to/from JSON.
type SerialisableEmitter interface {
	Emitter
	EmitterKind() string
}

// EmitterFunc is a function that is an Emitter.
type EmitterFunc func(groups []string, state *LexerState) Iterator

// Emit tokens for groups.
func (e EmitterFunc) Emit(groups []string, state *LexerState) Iterator {
	return e(groups, state)
}

type byGroupsEmitter struct {
	Emitters []Emitter `xml:"emit"`
}

func (b *byGroupsEmitter) EmitterKind() string { return "bygroups" }

func (b *byGroupsEmitter) Emit(groups []string, state *LexerState) Iterator {
	iterators := make([]Iterator, 0, len(groups)-1)
	if len(b.Emitters) != len(groups)-1 {
		iterators = append(iterators, Error.Emit(groups, state))
		// panic(errors.Errorf("number of groups %q does not match number of emitters %v", groups, emitters))
	} else {
		for i, group := range groups[1:] {
			if b.Emitters[i] != nil {
				iterators = append(iterators, b.Emitters[i].Emit([]string{group}, state))
			}
		}
	}
	return Concaterator(iterators...)
}

// ByGroups emits a token for each matching group in the rule's regex.
func ByGroups(emitters ...Emitter) Emitter {
	return &byGroupsEmitter{Emitters: emitters}
}

type byGroupNamesEmitter struct {
	Emitters map[string]Emitter `xml:"emitters"`
}

func (b *byGroupNamesEmitter) EmitterKind() string { return "bygroupnames" }

func (b *byGroupNamesEmitter) Emit(groups []string, state *LexerState) Iterator {
	iterators := make([]Iterator, 0, len(state.NamedGroups)-1)
	if len(state.NamedGroups)-1 == 0 {
		if emitter, ok := b.Emitters[`0`]; ok {
			iterators = append(iterators, emitter.Emit(groups, state))
		} else {
			iterators = append(iterators, Error.Emit(groups, state))
		}
	} else {
		ruleRegex := state.Rules[state.State][state.Rule].Regexp
		for i := 1; i < len(state.NamedGroups); i++ {
			groupName := ruleRegex.GroupNameFromNumber(i)
			group := state.NamedGroups[groupName]
			if emitter, ok := b.Emitters[groupName]; ok {
				if emitter != nil {
					iterators = append(iterators, emitter.Emit([]string{group}, state))
				}
			} else {
				iterators = append(iterators, Error.Emit([]string{group}, state))
			}
		}
	}
	return Concaterator(iterators...)
}

// ByGroupNames emits a token for each named matching group in the rule's regex.
func ByGroupNames(emitters map[string]Emitter) Emitter {
	return &byGroupNamesEmitter{Emitters: emitters}
}

// UsingByGroup emits tokens for the matched groups in the regex using a
// "sublexer". Used when lexing code blocks where the name of a sublexer is
// contained within the block, for example on a Markdown text block or SQL
// language block.
//
// The sublexer will be retrieved using sublexerGetFunc (typically
// internal.Get), using the captured value from the matched sublexerNameGroup.
//
// If sublexerGetFunc returns a non-nil lexer for the captured sublexerNameGroup,
// then tokens for the matched codeGroup will be emitted using the retrieved
// lexer. Otherwise, if the sublexer is nil, then tokens will be emitted from
// the passed emitter.
//
// Example:
//
// 	var Markdown = internal.Register(MustNewLexer(
// 		&Config{
// 			Name:      "markdown",
// 			Aliases:   []string{"md", "mkd"},
// 			Filenames: []string{"*.md", "*.mkd", "*.markdown"},
// 			MimeTypes: []string{"text/x-markdown"},
// 		},
// 		Rules{
// 			"root": {
// 				{"^(```)(\\w+)(\\n)([\\w\\W]*?)(^```$)",
// 					UsingByGroup(
// 						internal.Get,
// 						2, 4,
// 						String, String, String, Text, String,
// 					),
// 					nil,
// 				},
// 			},
// 		},
// 	))
//
// See the lexers/m/markdown.go for the complete example.
//
// Note: panic's if the number emitters does not equal the number of matched
// groups in the regex.
func UsingByGroup(sublexerGetFunc func(string) Lexer, sublexerNameGroup, codeGroup int, emitters ...Emitter) Emitter {
	return EmitterFunc(func(groups []string, state *LexerState) Iterator {
		// bounds check
		if len(emitters) != len(groups)-1 {
			panic("UsingByGroup expects number of emitters to be the same as len(groups)-1")
		}

		// grab sublexer
		sublexer := sublexerGetFunc(groups[sublexerNameGroup])

		// build iterators
		iterators := make([]Iterator, len(groups)-1)
		for i, group := range groups[1:] {
			if i == codeGroup-1 && sublexer != nil {
				var err error
				iterators[i], err = sublexer.Tokenise(nil, groups[codeGroup])
				if err != nil {
					panic(err)
				}
			} else if emitters[i] != nil {
				iterators[i] = emitters[i].Emit([]string{group}, state)
			}
		}

		return Concaterator(iterators...)
	})
}

type usingEmitter struct {
	Lexer string `xml:"lexer,attr"`

	lexer Lexer `xml:"-"` // TODO: Look this up when deserialising.
}

func (u *usingEmitter) EmitterKind() string { return "using" }

func (u *usingEmitter) Emit(groups []string, state *LexerState) Iterator {
	it, err := u.lexer.Tokenise(&TokeniseOptions{State: "root", Nested: true}, groups[0])
	if err != nil {
		panic(err)
	}
	return it
}

// Using returns an Emitter that uses a given Lexer for parsing and emitting.
func Using(lexer Lexer) Emitter {
	return &usingEmitter{Lexer: lexer.Config().Name, lexer: lexer}
}

type usingSelfEmitter struct {
	State string `xml:"state,attr"`
}

func (u *usingSelfEmitter) EmitterKind() string { return "usingself" }

func (u *usingSelfEmitter) Emit(groups []string, state *LexerState) Iterator {
	it, err := state.Lexer.Tokenise(&TokeniseOptions{State: u.State, Nested: true}, groups[0])
	if err != nil {
		panic(err)
	}
	return it
}

// UsingSelf is like Using, but uses the current Lexer.
func UsingSelf(stateName string) Emitter {
	return &usingSelfEmitter{stateName}
}
