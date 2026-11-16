package domain

import "strconv"

func formatCanon(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
