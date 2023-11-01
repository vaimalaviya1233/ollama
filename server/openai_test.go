package server

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		name          string
		modelTemplate string
		msgs          []OpenAIMessage
		want          string
	}{
		// {
		// 	name:          "successful prompt generation",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
		// 	msgs: []OpenAIMessage{
		// 		{Role: "system", Content: "You are a wizard."},
		// 		{Role: "user", Content: "Where is my wand?"},
		// 		{Role: "assistant", Content: "Your wand is in the drawer."},
		// 	},
		// 	want: "You are a wizard. User: Where is my wand?\nAssistant: Your wand is in the drawer.",
		// },
		// {
		// 	name:          "just system",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:", // Missing closing "}"
		// 	msgs: []OpenAIMessage{
		// 		{Role: "system", Content: "You are a Hagrid."},
		// 	},
		// 	want: "You are a Hagrid. User: \nAssistant: ",
		// },
		// {
		// 	name:          "just user",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:", // Missing closing "}"
		// 	msgs: []OpenAIMessage{
		// 		{Role: "user", Content: "Im a wizard?"},
		// 	},
		// 	want: " User: Im a wizard?\nAssistant: ",
		// },
		// {
		// 	name:          "just assistant",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:", // Missing closing "}"
		// 	msgs: []OpenAIMessage{
		// 		{Role: "assistant", Content: "Yes, you are."},
		// 	},
		// 	want: " User: \nAssistant: Yes, you are.",
		// },
		// {
		// 	name:          "Sequence Starting with User",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
		// 	msgs: []OpenAIMessage{
		// 		{Role: "user", Content: "What is my quest?"},
		// 		{Role: "assistant", Content: "Your quest is to seek the Grail."},
		// 	},
		// 	want: " User: What is my quest?\nAssistant: Your quest is to seek the Grail.",
		// },
		// {
		// 	name:          "Sequence Starting with Assistant",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
		// 	msgs: []OpenAIMessage{
		// 		{Role: "assistant", Content: "Welcome to Hogwarts."},
		// 	},
		// 	want: " User: \nAssistant: Welcome to Hogwarts.",
		// },
		// {
		// 	name:          "Incomplete Sequence Ending with User",
		// 	modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
		// 	msgs: []OpenAIMessage{
		// 		{Role: "system", Content: "You are in a dark forest."},
		// 		{Role: "user", Content: "Which way to go?"},
		// 	},
		// 	want: "You are in a dark forest. User: Which way to go?\nAssistant: ",
		// },
		{
			name:          "Two Complete Sequences",
			modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
			msgs: []OpenAIMessage{
				{Role: "system", Content: "You have found a treasure map."},
				{Role: "user", Content: "What does it say?"},
				{Role: "assistant", Content: "It points to a location in the desert."},
				{Role: "system", Content: "A storm is coming."},
				{Role: "user", Content: "How should we prepare?"},
				{Role: "assistant", Content: "Gather supplies and find shelter."},
			},
			want: "You have found a treasure map. User: What does it say?\nAssistant:\nIt points to a location in the desert.A storm is coming. User: How should we prepare?\nAssistant:\nGather supplies and find shelter.",
		},
		{
			name:          "System and User Only",
			modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
			msgs: []OpenAIMessage{
				{Role: "system", Content: "The enemy army approaches."},
				{Role: "user", Content: "What is their strength?"},
			},
			want: "The enemy army approaches. User: What is their strength?\nAssistant:",
		},
		{
			name:          "User and Assistant Only",
			modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
			msgs: []OpenAIMessage{
				{Role: "user", Content: "Tell me a joke."},
				{Role: "assistant", Content: "Why do scientists not trust atoms? Because they make up everything."},
			},
			want: " User: Tell me a joke.\nAssistant:\nWhy do scientists not trust atoms? Because they make up everything.",
		},
		{
			name:          "Multiple Messages from Same Role",
			modelTemplate: "{{.System}} User: {{.Prompt}}\nAssistant:",
			msgs: []OpenAIMessage{
				{Role: "system", Content: "You are at a crossroads."},
				{Role: "user", Content: "I take the left path."},
				{Role: "user", Content: "Actually, I change my mind, I take the right path."},
				{Role: "assistant", Content: "You encounter a troll."},
			},
			want: "You are at a crossroads. User: I take the left path.\nAssistant: You encounter a troll.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyTemplate(tt.modelTemplate, tt.msgs)
			if err != nil {
				t.Error(err)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("testPrompt() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
