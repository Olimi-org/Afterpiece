package words

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Select selects n random words from the short word list.
func Select(n int) ([]string, error) {
	selected := make([]string, n)
	words := shortWords.Get()
	max := big.NewInt(int64(len(words)))
	for i := range n {
		j, err := rand.Int(rand.Reader, max)
		if err != nil {
			return nil, fmt.Errorf("wordlist.Select %d: %v", n, err)
		}
		selected[i] = words[j.Int64()]
	}
	return selected, nil
}
