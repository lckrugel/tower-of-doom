package engine

import (
	"fmt"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine/entities"
)

// itemFromAsset builds a runtime item from its asset, resolving any granted
// move IDs through the engine's registry. This shared constructor is used by
// rewards, equipment, and merchant stock generation; it consumes no RNG.
func (e *Engine) itemFromAsset(asset game.ItemAsset) entities.Item {
	granted := make([]entities.Move, 0, len(asset.Moves))
	for _, id := range asset.Moves {
		moveAsset, ok := e.reg.MoveByID(id)
		if !ok {
			panic(fmt.Sprintf("unreachable: item %q references unknown move %q despite passing content validation",
				asset.ID, id))
		}
		granted = append(granted, entities.NewMoveFromAsset(moveAsset))
	}
	return entities.NewItemFromAsset(asset, granted...)
}
