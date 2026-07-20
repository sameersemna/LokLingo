package services

import "context"

type TranslationInput struct {
	Text   string
	Source string
	Target string
	Ctx    context.Context
}

type TranslationService interface {
	Translate(input TranslationInput) (string, error)
}
