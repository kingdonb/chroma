package chroma

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleSerialisation(t *testing.T) {
	tests := []Rule{
		Include("String"),
		{`\d+`, Text, nil},
		{`"`, String, Push("String")},
	}
	for _, test := range tests {
		data, err := xml.Marshal(test)
		require.NoError(t, err)
		t.Log(string(data))
		actual := Rule{}
		err = xml.Unmarshal(data, &actual)
		require.NoError(t, err)
		require.Equal(t, test, actual)
	}
}

func TestRulesSerialisation(t *testing.T) {
	expected := Rules{
		"root": {
			{`{{(- )?/\*(.|\n)*?\*/( -)?}}`, CommentMultiline, nil},
			{`{{[-]?`, CommentPreproc, Push("template")},
			{`[^{]+`, Other, nil},
			{`{`, Other, nil},
		},
		"template": {
			{`[-]?}}`, CommentPreproc, Pop(1)},
			{`(?=}})`, CommentPreproc, Pop(1)}, // Terminate the pipeline
			{`\(`, Operator, Push("subexpression")},
			{`"(\\\\|\\"|[^"])*"`, LiteralString, nil},
			Include("expression"),
		},
		"subexpression": {
			{`\)`, Operator, Pop(1)},
			Include("expression"),
		},
		"expression": {
			{`\s+`, Whitespace, nil},
			{`\(`, Operator, Push("subexpression")},
			{`(range|if|else|while|with|template|end|true|false|nil|and|call|html|index|js|len|not|or|print|printf|println|urlquery|eq|ne|lt|le|gt|ge)\b`, Keyword, nil},
			{`\||:?=|,`, Operator, nil},
			{`[$]?[^\W\d]\w*`, NameOther, nil},
			{`\$|[$]?\.(?:[^\W\d]\w*)?`, NameAttribute, nil},
			{`"(\\\\|\\"|[^"])*"`, LiteralString, nil},
			{`-?\d+i`, LiteralNumber, nil},
			{`-?\d+\.\d*([Ee][-+]\d+)?i`, LiteralNumber, nil},
			{`\.\d+([Ee][-+]\d+)?i`, LiteralNumber, nil},
			{`-?\d+[Ee][-+]\d+i`, LiteralNumber, nil},
			{`-?\d+(\.\d+[eE][+\-]?\d+|\.\d*|[eE][+\-]?\d+)`, LiteralNumberFloat, nil},
			{`-?\.\d+([eE][+\-]?\d+)?`, LiteralNumberFloat, nil},
			{`-?0[0-7]+`, LiteralNumberOct, nil},
			{`-?0[xX][0-9a-fA-F]+`, LiteralNumberHex, nil},
			{`-?0b[01_]+`, LiteralNumberBin, nil},
			{`-?(0|[1-9][0-9]*)`, LiteralNumberInteger, nil},
			{`'(\\['"\\abfnrtv]|\\x[0-9a-fA-F]{2}|\\[0-7]{1,3}|\\u[0-9a-fA-F]{4}|\\U[0-9a-fA-F]{8}|[^\\])'`, LiteralStringChar, nil},
			{"`[^`]*`", LiteralString, nil},
		},
	}
	data, err := xml.MarshalIndent(expected, "  ", "  ")
	require.NoError(t, err)
	re := regexp.MustCompile(`></[a-zA-Z]+>`)
	data = re.ReplaceAll(data, []byte(`/>`))
	fmt.Println(string(data))
	b := &bytes.Buffer{}
	w := gzip.NewWriter(b)
	fmt.Fprintln(w, string(data))
	w.Close()
	fmt.Println(len(data), b.Len())
	actual := Rules{}
	err = xml.Unmarshal(data, &actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
