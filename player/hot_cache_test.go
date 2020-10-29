package player

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHotCache_Get(t *testing.T) {
	hc := Init(1000, 1*time.Minute)
	assert.NotNil(t, hc)

	r := hc.Get("test")
	assert.Nil(t, r)

	chunk := reflectedChunk{body: []byte{1, 2, 3, 4}}
	hc.Set("test", &chunk)
	r = hc.Get("test")
	assert.NotNil(t, r)
	assert.EqualValues(t, chunk, *r)
}

func TestHotCache_Set(t *testing.T) {
	hc := Init(1000, 1*time.Minute)
	assert.NotNil(t, hc)

	chunk := reflectedChunk{body: []byte{1, 2, 3, 4}}
	hc.Set("test", &chunk)
	r := hc.Get("test")
	assert.NotNil(t, r)
	assert.EqualValues(t, chunk, *r)
}

func TestInit(t *testing.T) {
	hc := Init(1000, 1*time.Minute)
	assert.NotNil(t, hc)
}
