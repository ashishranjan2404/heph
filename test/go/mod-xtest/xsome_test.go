package xtest_test

import (
	"github.com/stretchr/testify/assert"
	xtest "mod-simple"
	"testing"
)

func TestXSanity(t *testing.T) {
	xtest.Hello()

	// :)
	assert.Equal(t, 1, 1)
}
