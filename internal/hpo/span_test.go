package hpo

import (
	"context"
	"testing"
)

func TestOriginalUnicodeWhitespaceOffsets(t *testing.T) {
	text := " \n\t患者否认癫痫发作。"
	idx := NewIndex(map[string]Term{"HP:0001250": {ID: "HP:0001250", Name: "Seizure", NameCN: "癫痫发作"}})
	result, err := NewExtractor(idx, nil).Extract(context.Background(), text, ExtractOptions{AllowLocalFallback: true})
	if err != nil || len(result.Terms) != 1 {
		t.Fatalf("literal extract: %+v %v", result, err)
	}
	item := result.Terms[0]
	if string([]rune(text)[item.SpanStart:item.SpanEnd]) != item.OriginalSymptom || item.SpanStart != 7 || item.Polarity != PolarityAbsent {
		t.Fatalf("offset not anchored to original input: %+v", item)
	}
}
