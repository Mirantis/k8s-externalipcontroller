package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventualCondition(t *testing.T) {
	fakeTest := &testing.T{}
	assert.False(t,
		EventualCondition(fakeTest, time.Millisecond*100, func() bool {
			return 2 == 3
		}, "test"),
	)
}
