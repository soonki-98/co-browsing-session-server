package serialnumber

import (
	"math/rand"
	"time"
)

// Generator는 새로운 SerialNumber를 발급하는 port다.
// 테스트에서는 결정적 구현으로 교체할 수 있다.
type Generator interface {
	Generate(length int) SerialNumber
}

const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ0123456789"

type randomGenerator struct {
	rng *rand.Rand
}

func NewRandomGenerator() Generator {
	return &randomGenerator{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (g *randomGenerator) Generate(length int) SerialNumber {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[g.rng.Intn(len(charset))]
	}
	return SerialNumber(b)
}
