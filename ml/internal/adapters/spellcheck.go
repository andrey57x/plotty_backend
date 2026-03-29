package adapters

import (
	"context"
	"fmt"

	"github.com/fivecode/plotty/internal/infrastructure/languagetool"
	"github.com/fivecode/plotty/ml/internal/models"
)

type LanguageToolAdapter struct {
	client *languagetool.Client
}

func NewLanguageToolAdapter(client *languagetool.Client) *LanguageToolAdapter {
	return &LanguageToolAdapter{
		client: client,
	}
}

func (a *LanguageToolAdapter) CheckText(ctx context.Context, text string) (models.SpellcheckResult, error) {
	resp, err := a.client.Check(ctx, text)
	if err != nil {
		return models.SpellcheckResult{}, err
	}

	res := models.SpellcheckResult{
		Summary: fmt.Sprintf("Найдено возможных ошибок: %d", len(resp.Matches)),
		Items:   make([]models.SpellcheckItem, 0, len(resp.Matches)),
	}

	runeText := []rune(text)

	for _, m := range resp.Matches {
		suggestion := ""
		if len(m.Replacements) > 0 {
			suggestion = m.Replacements[0].Value
		}

		fragment := ""
		if m.Offset+m.Length <= len(runeText) {
			fragment = string(runeText[m.Offset : m.Offset+m.Length])
		}

		res.Items = append(res.Items, models.SpellcheckItem{
			FragmentText: fragment,
			StartOffset:  m.Offset,
			EndOffset:    m.Offset + m.Length,
			Message:      m.Message,
			Suggestion:   suggestion,
		})
	}

	return res, nil
}
