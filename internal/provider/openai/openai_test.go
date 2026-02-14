package openai

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ListModels(t *testing.T) {

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping test")
	}

	config := Config{
		ID:      "test-openai",
		APIKey:  apiKey,
		BaseURL: "https://api.vveai.com/v1",
		//BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	require.NoError(t, err, "Failed to create OpenAI provider")
	require.NotNil(t, provider, "Provider should not be nil")

	t.Run("ListModels should return models", func(t *testing.T) {
		models, err := provider.ListModels(ctx)

		assert.NoError(t, err, "ListModels should not return error")

		assert.NotNil(t, models, "Models should not be nil")
		assert.Greater(t, len(models), 0, "Should return at least one model")

		pretty.Println(models)

		for _, model := range models {
			assert.NotEmpty(t, model.ID, "Model ID should not be empty")
			assert.NotEmpty(t, model.Name, "Model Name should not be empty")

			assert.Equal(t, model.ID, model.Name, "Model Name should equal ID")
		}

		modelIDs := make(map[string]bool)
		for _, model := range models {
			modelIDs[model.ID] = true
		}

		commonModels := []string{"gpt-4", "gpt-3.5-turbo", "gpt-4-turbo", "gpt-4o", "gpt-4o-mini"}
		hasCommonModel := false
		for _, commonModel := range commonModels {
			if modelIDs[commonModel] {
				hasCommonModel = true
				break
			}
		}
		assert.True(t, hasCommonModel, "Should contain at least one common model")
	})

}

func TestProvider_ListModels_ResponseStructure(t *testing.T) {

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping test")
	}

	config := Config{
		ID:           "test-openai-structure",
		APIKey:       apiKey,
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
	}

	ctx := context.Background()
	provider, err := NewProvider(ctx, config)
	require.NoError(t, err)

	models, err := provider.ListModels(ctx)
	require.NoError(t, err)
	require.NotNil(t, models)

	t.Run("Model structure validation", func(t *testing.T) {
		for i, model := range models {
			t.Run(fmt.Sprintf("Model_%d_%s", i, model.ID), func(t *testing.T) {

				assert.NotEmpty(t, model.ID, "Model ID should not be empty")
				assert.Greater(t, len(model.ID), 0, "Model ID should have length > 0")

				assert.NotEmpty(t, model.Name, "Model Name should not be empty")

				assert.Equal(t, model.ID, model.Name, "Model Name should equal ID")

				assert.Regexp(t, `^[a-zA-Z0-9._-]+$`, model.ID, "Model ID should match expected format")
			})
		}
	})
}
