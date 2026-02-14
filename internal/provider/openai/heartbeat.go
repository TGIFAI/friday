package openai

import (
	"context"
	"time"

	"github.com/tgifai/friday/internal/provider"
)

func (p *Provider) startHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		ctx := context.Background()
		select {
		case <-p.closeCh:

			return
		case <-ticker.C:

			p.checkAvailability(ctx)
		}
	}
}

func (p *Provider) checkAvailability(ctx context.Context) {
	models, err := p.ListModels(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	if err != nil {

		p.isAvailable = false
		p.availableModels = make([]provider.ModelInfo, 0)
		return
	}

	p.isAvailable = true
	p.availableModels = models
}
