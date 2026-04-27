package harvester

import (
	"fmt"

	"github.com/Joncik91/inflate/internal/config"
)

// CollectProfile renders the user profile as a context block.
func CollectProfile(p config.Profile) string {
	return fmt.Sprintf("Identity: %s\nWork: %s\nStyle preference: %s",
		p.Identity, p.Work, p.Style)
}
