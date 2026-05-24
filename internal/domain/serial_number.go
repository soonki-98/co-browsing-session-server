package domain

import (
	"math/rand"
	"time"
)

type SerialNumber string

func GenerateRandomSerialNumber(length int) SerialNumber {
	const CHARSET = "ABCDEFGHJKLMNPQRSTUVWXYZ0123456789"

	// 시리얼 번호 생성 로직 (예: 랜덤 문자열 생성)
	var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)

	for i := range b {
		b[i] = CHARSET[seededRand.Intn(len(CHARSET))]
	}

	return SerialNumber(b)
}
