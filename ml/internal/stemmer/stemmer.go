package stemmer

import (
	"regexp"
	"strings"

	"github.com/kljensen/snowball"
)

// Регулярное выражение для поиска всех слов в тексте (кириллица, латиница, цифры)
var WordRegex = regexp.MustCompile(`[а-яА-Яa-zA-Z0-9]+`)

// CleanAndStemToken очищает слово от пробелов, приводит к нижнему регистру
// и возвращает его основу (стем) для русского языка.
func CleanAndStemToken(word string) string {
	cleaned := strings.ToLower(strings.TrimSpace(word))
	// Игнорируем слишком короткие слова (меньше 3 букв)
	if len([]rune(cleaned)) < 3 {
		return ""
	}
	// Применяем русский стеммер Портера
	stemmed, err := snowball.Stem(cleaned, "russian", false)
	if err != nil {
		// В случае маловероятной ошибки возвращаем исходное очищенное слово
		return cleaned
	}
	return stemmed
}

// ExtractStemsFromText извлекает все уникальные основы слов из произвольного текста.
func ExtractStemsFromText(text string) map[string]struct{} {
	stems := make(map[string]struct{})
	words := WordRegex.FindAllString(text, -1)
	for _, w := range words {
		if stem := CleanAndStemToken(w); stem != "" {
			stems[stem] = struct{}{}
		}
	}
	return stems
}
