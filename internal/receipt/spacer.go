package receipt

import "fmt"

// Spacer inserts vertical blank space, Height dots tall.
type Spacer struct {
	Height int
}

// Validate reports whether s is well-formed: Height must not be negative.
func (s Spacer) Validate() error {
	if s.Height < 0 {
		return fmt.Errorf("spacer: height must be non-negative, got %d", s.Height)
	}
	return nil
}
