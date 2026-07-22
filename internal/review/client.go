package review

import (
	"errors"

	"github.com/nasraldin/camunda-lab/internal/ai"
)

// NewConfiguredClient is a composition-root helper. Review orchestration itself
// depends only on ai.ChatClient and never selects a provider.
func NewConfiguredClient(provider, model string, secrets ai.Secrets) (ai.ChatClient, error) {
	return ai.NewChatClientFromSecrets(provider, model, secrets)
}

// ValidateClientConfiguration validates provider, model, and endpoint without
// treating absent credentials as valid configuration.
func ValidateClientConfiguration(provider, model string, secrets ai.Secrets) error {
	validationSecrets := secrets
	validationSecrets.OpenAIKey = "validation-only"
	validationSecrets.AnthropicKey = "validation-only"
	_, err := ai.NewChatClientFromSecrets(provider, model, validationSecrets)
	return err
}

// IsMissingCredentials identifies the only configuration failure optional AI
// may downgrade to skipped/partial.
func IsMissingCredentials(err error) bool {
	var configError *ai.ConfigError
	return errors.As(err, &configError) && configError.Field == "api key"
}
