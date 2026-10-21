package prediction

// Gate reports one shadow trigger after three consecutive high recorded scores.
// A low, unavailable, or synthetic score resets the streak.
type Gate struct {
	consecutive int
	triggered   bool
}

func (g *Gate) Observe(available bool, source string, score float64) bool {
	if !available || source != "recorded" || !finite(score) || score < .8 || score > 1 {
		g.consecutive = 0
		g.triggered = false
		return false
	}
	g.consecutive++
	if g.consecutive >= 3 && !g.triggered {
		g.triggered = true
		return true
	}
	return false
}
