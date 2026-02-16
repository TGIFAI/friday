package telegram

import (
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
)

// mediaGroupDebounce is how long we wait after the last update in a media
// group before flushing it. Telegram typically delivers all items within
// 100-300ms, so 500ms is a safe window.
const mediaGroupDebounce = 500 * time.Millisecond

// pendingMediaGroup collects updates that belong to the same MediaGroupID.
type pendingMediaGroup struct {
	timer   *time.Timer
	chat    models.Chat
	from    *models.User
	caption string
	// captionEntities from the update that carried the caption.
	captionEntities []models.MessageEntity
	// photos stores the best PhotoSize per update (largest resolution).
	photos []models.PhotoSize
	// firstMessageID is the ID of the first update, used as the merged message ID.
	firstMessageID int
	// mentioned tracks whether any update in the group had an @bot mention.
	mentioned bool
}

// mediaGroupAggregator buffers incoming media-group updates and flushes
// them as a single batch after a debounce window.
type mediaGroupAggregator struct {
	mu     sync.Mutex
	groups map[string]*pendingMediaGroup // key: MediaGroupID
	onFlush func(g *pendingMediaGroup)    // called when debounce fires
}

func newMediaGroupAggregator(onFlush func(g *pendingMediaGroup)) *mediaGroupAggregator {
	return &mediaGroupAggregator{
		groups:  make(map[string]*pendingMediaGroup),
		onFlush: onFlush,
	}
}

// add appends a photo update to the pending group. Returns true if the
// update was consumed (caller should not process it further).
func (a *mediaGroupAggregator) add(msg *models.Message, mentioned bool) {
	groupID := msg.MediaGroupID

	a.mu.Lock()
	defer a.mu.Unlock()

	pg, exists := a.groups[groupID]
	if !exists {
		pg = &pendingMediaGroup{
			chat:           msg.Chat,
			from:           msg.From,
			firstMessageID: msg.ID,
		}
		a.groups[groupID] = pg
	}

	// Capture caption from whichever update carries it (usually the first).
	if msg.Caption != "" && pg.caption == "" {
		pg.caption = msg.Caption
		pg.captionEntities = msg.CaptionEntities
	}

	// Track mention across all updates in the group.
	if mentioned {
		pg.mentioned = true
	}

	// Pick the largest photo size from this update.
	if len(msg.Photo) > 0 {
		best := msg.Photo[len(msg.Photo)-1]
		pg.photos = append(pg.photos, best)
	}

	// Reset (or start) the debounce timer.
	if pg.timer != nil {
		pg.timer.Stop()
	}
	pg.timer = time.AfterFunc(mediaGroupDebounce, func() {
		a.flush(groupID)
	})
}

// flush removes the group from the map and calls onFlush.
func (a *mediaGroupAggregator) flush(groupID string) {
	a.mu.Lock()
	pg, exists := a.groups[groupID]
	if exists {
		delete(a.groups, groupID)
	}
	a.mu.Unlock()

	if exists && a.onFlush != nil {
		a.onFlush(pg)
	}
}
