package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmorganca/ollama/api"
)

var errInvalidRole = errors.New("invalid message role")

func promptFromRequestParams(c *gin.Context, model *Model, req api.GenerateRequest) (string, error) {
	if req.Template != "" {
		// override the default model template
		model.Template = req.Template
	}

	var prompt strings.Builder
	if req.Context != nil {
		// TODO: context is deprecated, at some point the context logic within this conditional should be removed
		prevCtx, err := loaded.runner.Decode(c.Request.Context(), req.Context)
		if err != nil {
			return "", err
		}

		// Remove leading spaces from prevCtx if present
		prevCtx = strings.TrimPrefix(prevCtx, " ")
		prompt.WriteString(prevCtx)
	}
	p, err := model.Prompt(&PromptVars{
		System: req.System,
		Prompt: req.Prompt,
	})
	if err != nil {
		return "", err
	}
	prompt.WriteString(p)
	return prompt.String(), nil
}

func promptFromMessages(model *Model, messages []api.Message) (string, error) {
	flush := func(vars *PromptVars, model *Model, prompt *strings.Builder) error {
		p, err := model.Prompt(vars)
		if err != nil {
			return err
		}
		prompt.WriteString(p)

		vars.Prompt = ""
		vars.System = ""
		return nil
	}

	var prompt strings.Builder
	vars := &PromptVars{}
	for _, m := range messages {
		if (m.Role == "system" || m.Role == "user") && vars.Prompt != "" {
			if err := flush(vars, model, &prompt); err != nil {
				return "", err
			}
		}

		if m.Role == "assistant" && (vars.Prompt != "" || vars.System != "") {
			if err := flush(vars, model, &prompt); err != nil {
				return "", err
			}
		}

		switch m.Role {
		case "system":
			vars.System = m.Content
		case "user":
			vars.Prompt = m.Content
		case "assistant":
			prompt.WriteString(m.Content)
		default:
			return "", fmt.Errorf("%w %q, role must be one of [system, user, assistant]", errInvalidRole, m.Role)
		}
	}

	if vars.Prompt != "" || vars.System != "" {
		if err := flush(vars, model, &prompt); err != nil {
			return "", err
		}
	}

	return prompt.String(), nil
}

type Predict struct {
	Model            *Model
	Prompt           string
	Format           string
	SendContext      bool
	CheckpointStart  time.Time
	CheckpointLoaded time.Time
	SessionDuration  time.Duration
	ResponseCallback func(*api.PredictResponse) // this function is used to transform the response into a custom format
}

func (p *Predict) Run(ctx context.Context) (chan any, *strings.Builder) {
	ch := make(chan any)
	var generated strings.Builder

	go func() {
		defer close(ch)

		fn := func(r api.PredictResponse) {
			// Update model expiration
			loaded.expireAt = time.Now().Add(p.SessionDuration)
			loaded.expireTimer.Reset(p.SessionDuration)

			r.Model = p.Model.Name
			r.CreatedAt = time.Now().UTC()

			// Build up the full response
			if _, err := generated.WriteString(r.Response); err != nil {
				ch <- gin.H{"error": err.Error()}
				return
			}

			if r.Done {
				r.TotalDuration = time.Since(p.CheckpointStart)
				r.LoadDuration = p.CheckpointLoaded.Sub(p.CheckpointStart)

				if p.SendContext {
					embd, err := loaded.runner.Encode(ctx, p.Prompt+generated.String())
					if err != nil {
						ch <- gin.H{"error": err.Error()}
						return
					}
					r.Context = embd
				}
			}

			// Execute additional handler-specific logic
			if p.ResponseCallback != nil {
				p.ResponseCallback(&r)
			}

			ch <- r
		}

		// Start prediction
		if err := loaded.runner.Predict(ctx, p.Prompt, p.Format, fn); err != nil {
			ch <- gin.H{"error": err.Error()}
		}
	}()

	return ch, &generated
}
